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
	if len(name) < socketNameLen {
		name += strings.Repeat("p", socketNameLen-len(name))
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
	return l, nil
}

const (
	// dialTimeout bounds how long Dial waits for the broker to come up.
	dialTimeout = 5 * time.Second
	// dialInterval is how often Dial retries while waiting.
	dialInterval = 20 * time.Millisecond
)

// Dial connects to the attach socket at path and completes the handshake.
//
// It retries until the socket answers, the context is done, or dialTimeout
// elapses. containerd spawns the logging process while creating the task and
// does not wait for it, so a client that dials straight after
// container.NewTask can arrive before the broker has bound its socket. For a
// container that is already running the first attempt succeeds.
func Dial(ctx context.Context, path string) (*Session, error) {
	ctx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()

	var d net.Dialer
	for {
		conn, err := d.DialContext(ctx, "unix", path)
		if err == nil {
			// A connection can sit in the listener's backlog before the broker
			// accepts it, so the handshake read needs a deadline of its own:
			// a broker that died between Listen and Serve would otherwise hang
			// the client for good.
			if deadline, ok := ctx.Deadline(); ok {
				if err := conn.SetReadDeadline(deadline); err != nil {
					conn.Close()
					return nil, err
				}
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
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("failed to connect to %s: %w", path, err)
		case <-time.After(dialInterval):
		}
	}
}
