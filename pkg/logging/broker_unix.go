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

package logging

import (
	"context"
	"errors"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/attachmux"
	"github.com/containerd/nerdctl/v2/pkg/cioutil"
)

// startBroker makes this process the owner of the container's stdio: it listens
// on the container's attach socket and opens the write end of the container's
// stdin FIFO.
//
// It returns a tee to feed raw container output to, and a stop function to call
// once the container has exited. When the broker cannot be set up the tee is nil
// and stop is a no-op: the container keeps logging rather than failing outright.
// A broker that cannot start is never a reason to fail the container. Sessions
// then cannot attach at all, and say so, because the task's stdio is already
// bound to this process and no client can reach it directly.
//
// Nothing here is recorded on disk: the socket path is a pure function of the
// data store, the namespace and the container ID, and a client derives it the
// same way.
func startBroker(ctx context.Context, dataStore, ns, id string, tty bool) (func(stream string, p []byte), func(exited bool)) {
	noop := func(bool) {}

	socketPath := attachmux.SocketPath(dataStore, ns, id)
	listener, err := attachmux.Listen(socketPath)
	if err != nil {
		log.G(ctx).WithError(err).Warn("failed to listen on the attach socket, multi-session attach is disabled")
		return nil, noop
	}

	broker := attachmux.NewBroker(tty, nil)

	serveCtx, cancelServe := context.WithCancel(ctx)
	go func() {
		if err := broker.Serve(serveCtx, listener); err != nil {
			log.G(ctx).WithError(err).Warn("the attach socket stopped accepting sessions")
		}
	}()

	// Stdin is opened off this path, for two reasons. The shim opens the read
	// end of the FIFO only after it has forked the runtime, while this process
	// was spawned earlier, so the open has to be retried; and blocking here
	// would stall the caller before it starts reading the container's output.
	go func() {
		stdinPath := cioutil.StdinFIFOPath(dataStore, ns, id)
		w, err := openStdinWriter(serveCtx, stdinPath)
		if err != nil {
			log.G(ctx).WithError(err).Debugf("the container has no stdin at %q, attached sessions are output only", stdinPath)
			return
		}
		if !broker.SetStdin(w) {
			w.Close()
		}
	}()

	tee := func(stream string, p []byte) {
		switch stream {
		case streamStdout:
			broker.Write(attachmux.StreamStdout, p)
		case streamStderr:
			broker.Write(attachmux.StreamStderr, p)
		}
	}

	// stop is called twice: once by loggingProcessAdapter as soon as the
	// container's output has been read, so that sessions learn about the exit
	// without waiting for the log driver, and once by the deferred call in
	// loggerFunc. Only the first call decides what the sessions are told.
	var stopOnce sync.Once
	stop := func(exited bool) {
		stopOnce.Do(func() {
			// The listener goes first. A session that connected between
			// Close and here would be greeted by a broker that is already
			// closed, get its connection dropped without a hello, and have no
			// way to tell that from a broken broker.
			//
			// Closing the listener also removes the socket file, but only while
			// it is still the one this process created.
			cancelServe()
			listener.Close()
			// Close releases the container's stdin along with the sessions.
			broker.Close(exited)
		})
	}

	return tee, stop
}

const (
	// stdinOpenTimeout bounds how long openStdinWriter waits for the shim to
	// open the read end of the container's stdin FIFO.
	stdinOpenTimeout = 30 * time.Second
	// stdinOpenInterval is how often it retries while waiting.
	stdinOpenInterval = 20 * time.Millisecond
)

// openStdinWriter opens the write end of the container's stdin FIFO, or reports
// that the container has no stdin.
//
// Opening a FIFO O_WRONLY|O_NONBLOCK fails with ENXIO for as long as no reader
// has it open, and that is the only reliable signal available here. The FIFO
// file outlives the task that used it, so its presence says nothing: a
// container restarted without -i leaves the previous run's FIFO behind, and
// containerd's restart monitor recreates a terminal task through
// cio.TerminalLogURI, whose config carries no Stdin (plugins/restart/change.go),
// so the shim never opens the read end at all.
//
// github.com/containerd/fifo cannot answer this. Given O_NONBLOCK it strips the
// flag and performs the blocking open in a goroutine, returning a handle
// straight away and documenting that "read/write will be connected after the
// actual fifo is open". Handing that to the broker would make every container
// look as though it had stdin, and the first write from a session would block
// forever.
//
// The file keeps O_NONBLOCK. Go registers a FIFO opened that way with the
// runtime poller, which gives two things the broker depends on: Write blocks
// the goroutine until the pipe has room instead of returning EAGAIN, and
// Close evicts a goroutine already blocked in Write instead of waiting for it,
// which is what lets Broker.Close return while a container that stopped reading
// its stdin has a session's write outstanding.
//
// os/file_unix.go opts fifos out of kqueue on darwin and ios, so neither would
// hold there; this file is built only for linux and freebsd.
func openStdinWriter(ctx context.Context, path string) (*os.File, error) {
	deadline := time.Now().Add(stdinOpenTimeout)
	for {
		f, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, syscall.ENXIO) || time.Now().After(deadline) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(stdinOpenInterval):
		}
	}
}
