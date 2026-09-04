//go:build linux || freebsd

/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package attachmux

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// probeSeq keeps two concurrent probes inside one process off each other's
// socket path. Different processes are separated by the pid in the name.
var probeSeq atomic.Uint64

// Probe reports whether a container's attach socket directory can be created
// and bound in.
//
// nerdctl has to decide whether to hand a container's stdio to the logging
// process before the task exists, but whether the broker actually came up is
// only known afterwards. Probing first turns the common failures into a clean
// fallback to the legacy path, instead of a container whose output nobody can
// show.
func Probe(dataStore string) error {
	dir := socketDir(dataStore)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// Bind a real socket rather than creating a regular file: a file proves
	// neither that the filesystem supports unix sockets nor that the path fits
	// in sockaddr_un.sun_path, which a long data store path can break.
	//
	// The name is the same length as a real socket name so that the sun_path
	// check is representative. It carries the pid so that concurrent nerdctl
	// invocations do not probe over each other, and a counter so that two
	// concurrent calls inside one process do not either. Real names are hex
	// digests, so a name starting with "probe" cannot collide with one.
	name := fmt.Sprintf("probe%d-%d", os.Getpid(), probeSeq.Add(1))
	switch {
	case len(name) < socketNameLen:
		name += strings.Repeat("p", socketNameLen-len(name))
	case len(name) > socketNameLen:
		// A large pid plus the counter can overrun. Trim rather than probe with
		// a longer name than a real socket: the point is to test a path of
		// representative length, and a longer one would reject a data store
		// that in fact fits.
		name = name[:socketNameLen]
	}
	path := filepath.Join(dir, name+".sock")

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	l, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	// Closing a unix listener unlinks its socket.
	return l.Close()
}

// Listen creates the attach socket at path with mode 0600.
func Listen(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	// A container that was killed leaves its socket behind, and bind would then
	// fail with EADDRINUSE.
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0600); err != nil {
		l.Close()
		return nil, err
	}

	// Go removes the socket on Close by path, without checking that the file is
	// still the one it created. Two brokers can briefly overlap for a single
	// container: the restart monitor creates the new task, and with it a new
	// logging process, while the old one is still finishing its log driver. The
	// new broker taking the path over is correct, since it is the one attached
	// to the live task, but the old one exiting afterwards would then unlink
	// the live socket and leave the new broker listening on an inode nobody can
	// reach. So the removal is done here instead, against the inode.
	ul, ok := l.(*net.UnixListener)
	if !ok {
		return l, nil
	}
	ino, err := inodeOf(path)
	if err != nil {
		// Still Go's to unlink at this point, so closing cleans up.
		l.Close()
		return nil, err
	}
	// Taken over only once the inode is known, so that no failure above leaves
	// the socket on disk with nobody to remove it.
	ul.SetUnlinkOnClose(false)
	return &ownedListener{UnixListener: ul, path: path, ino: ino}, nil
}

// ownedListener removes its socket file on Close, but only while that file is
// still the one Listen created.
type ownedListener struct {
	*net.UnixListener
	path string
	ino  uint64
}

func (l *ownedListener) Close() error {
	err := l.UnixListener.Close()
	if ino, serr := inodeOf(l.path); serr == nil && ino == l.ino {
		os.Remove(l.path)
	}
	return err
}

func inodeOf(path string) (uint64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("attachmux: cannot read the inode of %s", path)
	}
	// Ino is uint64 on both platforms this file is built for.
	return st.Ino, nil
}

const (
	// dialTimeout bounds how long DialStarting waits for the broker to come up.
	dialTimeout  = 5 * time.Second
	dialInterval = 20 * time.Millisecond
)

// Dial connects to the attach socket at path and completes the handshake.
//
// It makes a single attempt. The caller is reaching a container that has been
// running for a while, so a socket that is missing or refuses the connection is
// proof that there is no broker to talk to, not a race worth waiting out.
func Dial(ctx context.Context, path string) (*Session, error) {
	return dial(ctx, path, false)
}

// DialStarting is Dial for a caller that has just created the task.
//
// containerd spawns the logging process while creating the task and does not
// wait for it, so for a short while neither a missing socket nor a refused
// connection says anything: the broker may still be binding, possibly over a
// file left behind by a previous one. This retries until the socket answers,
// the context is done, or dialTimeout elapses.
func DialStarting(ctx context.Context, path string) (*Session, error) {
	return dial(ctx, path, true)
}

func dial(ctx context.Context, path string, retry bool) (*Session, error) {
	if retry {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, dialTimeout)
		defer cancel()
	}

	var d net.Dialer
	for {
		conn, err := d.DialContext(ctx, "unix", path)
		if err == nil {
			return greet(ctx, conn)
		}
		if !retry {
			return nil, fmt.Errorf("failed to connect to %s: %w", path, err)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("failed to connect to %s: %w", path, err)
		case <-time.After(dialInterval):
		}
	}
}

// greet completes the handshake on an accepted connection.
func greet(ctx context.Context, conn net.Conn) (*Session, error) {
	// A connection can sit in the listener's backlog before the broker accepts
	// it, so the handshake read needs a deadline of its own: a broker that died
	// between Listen and Serve would otherwise hang the client for good.
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(dialTimeout)
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		conn.Close()
		return nil, err
	}
	session, err := NewSession(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}
	// Streaming has no deadline of its own.
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		session.Close()
		return nil, err
	}
	return session, nil
}

// RemoveSocket deletes an attach socket. Removing one that is not there is not
// an error.
func RemoveSocket(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
