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
	"io"
	"net"
	"sync"
	"time"

	"github.com/containerd/log"
)

const (
	// defaultQueueDepth is how many frames are buffered for a single session. A
	// session that falls this far behind is disconnected: the container's output
	// must never wait on a consumer.
	// https://github.com/containerd/nerdctl/issues/5137
	defaultQueueDepth = 256
	// writeTimeout bounds a single write to a session, so that a session that is
	// connected but no longer draining its socket is eventually dropped instead
	// of leaking a goroutine and its queue for the container's lifetime.
	writeTimeout = 30 * time.Second
	// closeDrainTimeout bounds how long Close waits for the writers to flush
	// what is already queued. containerd calls os.Exit as soon as the logging
	// function returns (core/runtime/v2/logging/logging_unix.go), so a writer
	// still draining at that point is killed mid-frame.
	closeDrainTimeout = 2 * time.Second
	// DrainTimeout bounds how long a client waits, after the container has
	// exited, for the broker to deliver what it still has queued.
	//
	// It has to be longer than closeDrainTimeout. A client that gave up first
	// would cancel its stream and close the socket while the broker was still
	// flushing, dropping the tail of the container's output and reporting
	// success for it.
	DrainTimeout = 3 * closeDrainTimeout
)

// Broker owns a container's stdio. It fans container output out to every
// connected session and merges the sessions' input into the container's stdin.
//
// Every method is safe for concurrent use, and none of them block on a session.
type Broker struct {
	tty bool

	mu       sync.Mutex
	sessions map[*session]struct{}
	closed   bool
	// stdin is the write end of the container's stdin, installed by NewBroker
	// or later by SetStdin, and taken back by Close. It is guarded by mu so
	// that closing and installing cannot interleave.
	stdin io.WriteCloser

	// writers tracks the per-session writer goroutines so that Close can flush
	// what is queued before the process is torn down.
	writers sync.WaitGroup

	// stdinWriteMu keeps a session's write to the container's stdin from being
	// interleaved with another session's. It guards the write, not the field:
	// a write to a FIFO the container is not reading blocks until the pipe
	// drains, and Close must be able to release the descriptor meanwhile
	// rather than queue behind it. stdin itself is guarded by mu.
	stdinWriteMu sync.Mutex
}

type session struct {
	conn   net.Conn
	frames chan []byte

	// mu guards closed together with the send on frames. Without it, Close
	// could send on a channel that a concurrent dropSession has just closed,
	// which panics even from inside a select.
	mu     sync.Mutex
	closed bool
	// exit is the final frame writeLoop delivers after frames has drained. See
	// closeWith.
	exit []byte
}

// send queues frame for the session. It reports false only when the session's
// queue is full, meaning the session has fallen behind and has to be dropped. A
// session that is already closed reports true: it is gone, not slow.
func (s *session) send(frame []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return true
	}
	select {
	case s.frames <- frame:
		return true
	default:
		return false
	}
}

// closeWith closes the session's queue, leaving exit for writeLoop to deliver
// once everything already queued has gone out. exit is nil when the session is
// being dropped rather than told the container is gone.
//
// The exit frame does not go through the queue. A session whose queue happens
// to be exactly full at that moment has received all of the container's output
// and is not behind; pushing the exit through the same bounded channel would
// drop it and leave the client reporting a lost connection for a container that
// finished normally.
func (s *session) closeWith(exit []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	s.exit = exit
	close(s.frames)
}

// close stops the session's writer. The writer closes the connection once it
// has drained whatever is already queued.
func (s *session) close() { s.closeWith(nil) }

// NewBroker returns a Broker fanning container output out to its sessions.
//
// stdin is the write end of the container's stdin and may be nil, either
// because the container has none or because it is not known yet; SetStdin
// installs it later. The Broker holds it open for the container's lifetime,
// which is what makes detaching leave the container running, and closes it only
// in Close.
//
// Note that closing this end would not deliver EOF to the container anyway.
// The shim opens the same FIFO O_WRONLY and holds that descriptor until
// task.CloseIO (cmd/containerd-shim-runc-v2/process/init.go, openStdin), so
// closing the container's stdin is a task operation, not a broker one.
func NewBroker(tty bool, stdin io.WriteCloser) *Broker {
	return &Broker{
		tty:      tty,
		stdin:    stdin,
		sessions: map[*session]struct{}{},
	}
}

// Write fans p out to every connected session. It never blocks: a session whose
// queue is full is disconnected instead. p is copied, so the caller is free to
// reuse it immediately.
func (b *Broker) Write(stream byte, p []byte) {
	if len(p) == 0 {
		return
	}

	b.mu.Lock()
	if b.closed || len(b.sessions) == 0 {
		b.mu.Unlock()
		return
	}
	frame, err := EncodeFrame(stream, p)
	if err != nil {
		b.mu.Unlock()
		log.L.WithError(err).Warn("attachmux: dropping an oversized output frame")
		return
	}
	var slow []*session
	for s := range b.sessions {
		if !s.send(frame) {
			slow = append(slow, s)
		}
	}
	for _, s := range slow {
		delete(b.sessions, s)
	}
	b.mu.Unlock()

	for _, s := range slow {
		log.L.Warn("attachmux: an attach session stopped keeping up, disconnecting it")
		s.close()
	}
}

// SessionCount returns how many sessions are currently connected.
func (b *Broker) SessionCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.sessions)
}

// Serve accepts sessions on l until ctx is done or l is closed.
func (b *Broker) Serve(ctx context.Context, l net.Listener) error {
	go func() {
		<-ctx.Done()
		l.Close()
	}()

	for {
		conn, err := l.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		b.addSession(conn)
	}
}

// Close disconnects every session and releases the container's stdin.
//
// exited says whether the container is known to have exited. Only then are the
// sessions told so, and they treat it as proof: `nerdctl attach` stops
// streaming and reads the exit code from containerd. The owner cannot infer it
// from its own stdio reaching EOF, because that also happens when the logging
// process is asked to shut down while the container keeps running, and a client
// told the container had gone would then wait for an exit that never comes.
//
// A false value tears the sessions down without that claim, and a client
// reports the session as broken, which is what it is.
//
// The exit carries no code: a session reads that from containerd.
func (b *Broker) Close(exited bool) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	sessions := make([]*session, 0, len(b.sessions))
	for s := range b.sessions {
		sessions = append(sessions, s)
	}
	b.sessions = map[*session]struct{}{}
	// Take the descriptor out under the same lock that SetStdin uses, so that
	// a concurrent SetStdin either loses the race and closes its own file, or
	// wins it and is closed here.
	stdin := b.stdin
	b.stdin = nil
	b.mu.Unlock()

	// The exit is handed over before stdin is released, and that order matters.
	// A readLoop blocked writing to a FIFO the container stopped reading wakes
	// up with an error the moment the descriptor closes, and its deferred
	// dropSession closes the session's queue; doing this afterwards would leave
	// the client reporting an unexpected disconnect for a container that exited
	// normally.
	var exit []byte
	if exited {
		frame, err := EncodeControl(Control{Type: ControlExit})
		if err != nil {
			log.L.WithError(err).Warn("attachmux: failed to encode the exit frame")
		} else {
			exit = frame
		}
	}
	for _, s := range sessions {
		s.closeWith(exit)
	}

	// Wait for the writers to flush. Closing the queues is not enough: the
	// caller is the logging process, and containerd calls os.Exit as soon as
	// the logging function returns, which would kill a writer mid-frame and
	// lose the container's last output along with the exit message.
	drained := make(chan struct{})
	go func() {
		b.writers.Wait()
		close(drained)
	}()
	select {
	case <-drained:
	case <-time.After(closeDrainTimeout):
		log.L.Warn("attachmux: timed out flushing attach sessions on close")
	}

	// Released last, outside the lock and outside stdinWriteMu: a readLoop can
	// be blocked in Write on a FIFO the container stopped reading, and waiting
	// for it would hang the logging process on SIGTERM. The FIFO is registered
	// with the runtime poller, so closing it unblocks that write with ErrClosed.
	if stdin != nil {
		stdin.Close()
	}
}

// SetStdin gives the broker the write end of the container's stdin once it is
// known to be usable, and reports whether the broker took it. It returns false
// for a broker that is already closed, and the caller then closes w itself.
//
// Stdin arrives late because whether the container has one at all can only be
// established by opening the FIFO, which the owner has to do off the path that
// reads the container's output. See pkg/logging/broker_unix.go.
func (b *Broker) SetStdin(w io.WriteCloser) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.stdin != nil {
		return false
	}
	b.stdin = w
	return true
}

// addSession registers conn, greets it and starts its two pumps.
func (b *Broker) addSession(conn net.Conn) {
	hello, err := EncodeControl(Control{Type: ControlHello, Version: ProtocolVersion, TTY: b.tty})
	if err != nil {
		conn.Close()
		return
	}

	s := &session{conn: conn, frames: make(chan []byte, defaultQueueDepth)}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		conn.Close()
		return
	}
	b.sessions[s] = struct{}{}
	// The hello has to be queued before anything else so that it is the first
	// frame the session sees.
	s.send(hello)
	// Add under the lock, before the session becomes reachable to Close.
	// Adding after unlocking would let Close observe the session, close it and
	// return from Wait with the counter still at zero, which both breaks the
	// sync.WaitGroup contract and lets the logging process exit before this
	// writer has flushed anything.
	b.writers.Add(1)
	b.mu.Unlock()

	go b.writeLoop(s)
	go b.readLoop(s)
}

// writeLoop drains the session's queue onto its connection.
func (b *Broker) writeLoop(s *session) {
	defer b.writers.Done()
	defer s.conn.Close()

	for frame := range s.frames {
		if err := s.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
			b.dropSession(s)
			return
		}
		if _, err := s.conn.Write(frame); err != nil {
			b.dropSession(s)
			return
		}
	}

	// Everything queued has gone out. If the container exited, say so now: this
	// is the last thing the session ever sees, and it is what tells the client
	// apart from a broker that vanished.
	s.mu.Lock()
	exit := s.exit
	s.mu.Unlock()
	if exit == nil {
		return
	}
	if err := s.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return
	}
	if _, err := s.conn.Write(exit); err != nil {
		log.L.WithError(err).Debug("attachmux: failed to deliver the exit frame")
	}
}

// readLoop forwards the session's input to the container.
func (b *Broker) readLoop(s *session) {
	defer b.dropSession(s)

	for {
		stream, payload, err := ReadFrame(s.conn)
		if err != nil {
			return
		}
		switch stream {
		case StreamStdin:
			// stdin can be installed by SetStdin after this loop has started,
			// and taken away by Close while it is running, so it is read under
			// mu. The write itself is held only by stdinWriteMu: taking mu
			// across a write that can block on a full FIFO would hang Close.
			b.mu.Lock()
			w := b.stdin
			b.mu.Unlock()
			if w == nil {
				continue
			}

			b.stdinWriteMu.Lock()
			_, err = w.Write(payload)
			b.stdinWriteMu.Unlock()
			if err != nil {
				// Close closed the descriptor under a blocked write, or the
				// container is gone. Either way this session's input has
				// nowhere to go.
				log.L.WithError(err).Debug("attachmux: failed to write to the container stdin")
				return
			}
		}
	}
}

// dropSession unregisters s and stops its writer.
func (b *Broker) dropSession(s *session) {
	b.mu.Lock()
	delete(b.sessions, s)
	b.mu.Unlock()
	s.close()
}
