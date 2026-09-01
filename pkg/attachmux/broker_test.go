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
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

// syncBuffer is an io.WriteCloser that records what the container's stdin
// received. It is safe for concurrent use because several sessions may write
// at once.
type syncBuffer struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	closed bool
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func (s *syncBuffer) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// connect wires a new session into b over an in-memory connection and consumes
// its hello message, returning the session's end of the pipe.
func connect(t *testing.T, b *Broker) net.Conn {
	t.Helper()

	mine, theirs := net.Pipe()
	t.Cleanup(func() { mine.Close() })

	go b.addSession(theirs)

	assert.NilError(t, mine.SetReadDeadline(time.Now().Add(5*time.Second)))
	stream, payload, err := ReadFrame(mine)
	assert.NilError(t, err)
	assert.Equal(t, stream, StreamControl)

	var c Control
	assert.NilError(t, json.Unmarshal(payload, &c))
	assert.Equal(t, c.Type, ControlHello)
	assert.Equal(t, c.Version, ProtocolVersion)

	// Leave a deadline armed for the rest of the test rather than clearing it.
	// net.Pipe refuses SetReadDeadline once the peer has closed, and the broker
	// closes its end as soon as a session is dropped, so a test cannot reliably
	// arm one later. This is only a guard against hanging.
	assert.NilError(t, mine.SetReadDeadline(time.Now().Add(30*time.Second)))
	return mine
}

// waitSessions blocks until the broker reports n sessions, or fails the test.
func waitSessions(t *testing.T, b *Broker, n int) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if b.SessionCount() == n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected %d sessions, got %d", n, b.SessionCount())
}

func TestBrokerFansOutToEverySession(t *testing.T) {
	t.Parallel()

	b := NewBroker(true, nil)
	first := connect(t, b)
	second := connect(t, b)
	waitSessions(t, b, 2)

	b.Write(StreamStdout, []byte("shared output"))

	for _, conn := range []net.Conn{first, second} {
		stream, payload, err := ReadFrame(conn)
		assert.NilError(t, err)
		assert.Equal(t, stream, StreamStdout)
		assert.Equal(t, string(payload), "shared output")
	}
}

func TestBrokerMergesStdinFromEverySession(t *testing.T) {
	t.Parallel()

	stdin := &syncBuffer{}
	b := NewBroker(false, stdin)
	first := connect(t, b)
	second := connect(t, b)
	waitSessions(t, b, 2)

	frame, err := EncodeFrame(StreamStdin, []byte("from-first\n"))
	assert.NilError(t, err)
	_, err = first.Write(frame)
	assert.NilError(t, err)

	frame, err = EncodeFrame(StreamStdin, []byte("from-second\n"))
	assert.NilError(t, err)
	_, err = second.Write(frame)
	assert.NilError(t, err)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		got := stdin.String()
		if len(got) == len("from-first\nfrom-second\n") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	got := stdin.String()
	assert.Assert(t, bytes.Contains([]byte(got), []byte("from-first\n")), "got %q", got)
	assert.Assert(t, bytes.Contains([]byte(got), []byte("from-second\n")), "got %q", got)
}

func TestBrokerSplitsAnOversizedWrite(t *testing.T) {
	t.Parallel()

	// Write is the entry point for whoever owns the container's stdio, and this
	// package does not get to assume the size of their read buffer. A chunk
	// larger than one frame is split, not dropped.
	b := NewBroker(true, nil)
	conn := connect(t, b)
	waitSessions(t, b, 1)

	payload := bytes.Repeat([]byte("x"), maxPayload+1000)
	go b.Write(StreamStdout, payload)

	var got []byte
	for len(got) < len(payload) {
		stream, chunk, err := ReadFrame(conn)
		assert.NilError(t, err)
		assert.Equal(t, stream, StreamStdout)
		got = append(got, chunk...)
	}
	assert.Equal(t, len(got), len(payload))
	assert.DeepEqual(t, got, payload)
}

func TestBrokerRefusesToWriteOnTheControlStream(t *testing.T) {
	t.Parallel()

	// Control is the broker's own channel: container output must not be able to
	// forge a hello or an exit.
	b := NewBroker(true, nil)
	conn := connect(t, b)
	waitSessions(t, b, 1)

	b.Write(StreamControl, []byte(`{"type":"exit"}`))
	b.Write(StreamStdout, []byte("real output"))

	stream, payload, err := ReadFrame(conn)
	assert.NilError(t, err)
	assert.Equal(t, stream, StreamStdout)
	assert.Equal(t, string(payload), "real output")
}

func TestBrokerDisconnectsASessionThatStopsReading(t *testing.T) {
	t.Parallel()

	// The container must never be held up by a session that stopped consuming.
	b := NewBroker(true, nil)
	stalled := connect(t, b)
	waitSessions(t, b, 1)

	// Never read from `stalled` again. Write far more than the session queue
	// can hold; every call must return promptly.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range defaultQueueDepth * 4 {
			b.Write(StreamStdout, []byte("x"))
		}
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Broker.Write blocked on a session that stopped reading")
	}

	waitSessions(t, b, 0)
	assert.Assert(t, stalled != nil)
}

func TestBrokerKeepsRunningWithNoSessions(t *testing.T) {
	t.Parallel()

	b := NewBroker(true, nil)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 1000 {
			b.Write(StreamStdout, []byte("output with nobody attached"))
		}
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Broker.Write blocked with no sessions attached")
	}
}

func TestBrokerDoesNotCloseStdinWhenASessionLeaves(t *testing.T) {
	t.Parallel()

	// Detaching must not send EOF to the container: this is what makes
	// ctrl-p ctrl-q leave a container running.
	stdin := &syncBuffer{}
	b := NewBroker(false, stdin)
	conn := connect(t, b)
	waitSessions(t, b, 1)

	assert.NilError(t, conn.Close())
	waitSessions(t, b, 0)

	assert.Equal(t, stdin.isClosed(), false)
}

func TestBrokerAcceptsStdinAfterASessionConnects(t *testing.T) {
	t.Parallel()

	// The owner opens the container's stdin FIFO off the path that reads its
	// output, so stdin can arrive after sessions are already streaming. A
	// session that connected first still has to reach the container once it
	// does.
	//
	// Input sent before stdin is installed is dropped rather than buffered,
	// which readLoop documents, but a test cannot pin that down: net.Pipe
	// reports a write as complete once the peer has read the bytes, not once
	// readLoop has acted on them, so SetStdin can always land in between.
	b := NewBroker(false, nil)
	conn := connect(t, b)
	waitSessions(t, b, 1)

	stdin := &syncBuffer{}
	assert.Equal(t, b.SetStdin(stdin), true)

	frame, err := EncodeFrame(StreamStdin, []byte("late"))
	assert.NilError(t, err)
	_, err = conn.Write(frame)
	assert.NilError(t, err)

	for range 100 {
		if stdin.String() == "late" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("stdin did not reach the broker, got %q", stdin.String())
}

// failingWriter models the container's stdin FIFO after the container stopped
// reading it: every write fails, the way a FIFO with no reader gives EPIPE.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, syscall.EPIPE }
func (failingWriter) Close() error              { return nil }

func TestBrokerKeepsAnOutputSessionAfterAFailedStdinWrite(t *testing.T) {
	t.Parallel()

	// A container that closed its stdin and keeps printing, or one that has
	// just exited, must not cost the session its output on the next keystroke.
	// Dropping the session there also loses the exit frame, which is exactly
	// the "unexpected disconnect is an error, not a detach" case.
	b := NewBroker(true, failingWriter{})
	conn := connect(t, b)
	waitSessions(t, b, 1)

	frame, err := EncodeFrame(StreamStdin, []byte("keystroke"))
	assert.NilError(t, err)
	_, err = conn.Write(frame)
	assert.NilError(t, err)

	// The session is still there, and still receives the container's output.
	b.Write(StreamStdout, []byte("still printing"))
	stream, payload, err := ReadFrame(conn)
	assert.NilError(t, err)
	assert.Equal(t, stream, StreamStdout)
	assert.Equal(t, string(payload), "still printing")
	assert.Equal(t, b.SessionCount(), 1)
}

func TestBrokerCloseReleasesStdin(t *testing.T) {
	t.Parallel()

	// The container has exited, so nothing can read its stdin any more. Leaving
	// the descriptor open would keep the FIFO alive for as long as the logging
	// process lives.
	stdin := &syncBuffer{}
	b := NewBroker(false, stdin)
	b.Close(true)
	assert.Equal(t, stdin.isClosed(), true)

	assert.Equal(t, b.SetStdin(&syncBuffer{}), false)
}

// blockingWriter models the container's stdin FIFO when the container has
// stopped reading it: the write blocks until the descriptor is closed.
type blockingWriter struct {
	released chan struct{}
	once     sync.Once
}

func newBlockingWriter() *blockingWriter {
	return &blockingWriter{released: make(chan struct{})}
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	<-w.released
	return 0, os.ErrClosed
}

func (w *blockingWriter) Close() error {
	w.once.Do(func() { close(w.released) })
	return nil
}

func TestBrokerCloseDoesNotWaitForABlockedStdinWrite(t *testing.T) {
	t.Parallel()

	// A container that stopped reading its stdin leaves a session's write
	// blocked on a full FIFO. Close has to release the descriptor rather than
	// queue behind that write: it runs on SIGTERM, and the logging process is
	// killed shortly after it returns.
	stdin := newBlockingWriter()
	b := NewBroker(false, stdin)
	conn := connect(t, b)
	waitSessions(t, b, 1)

	frame, err := EncodeFrame(StreamStdin, []byte("blocked"))
	assert.NilError(t, err)
	_, err = conn.Write(frame)
	assert.NilError(t, err)

	// Give readLoop time to reach the write before closing.
	time.Sleep(100 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		b.Close(true)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close blocked behind a stdin write")
	}

	// And the session still learns that the container exited. Releasing stdin
	// wakes the blocked readLoop, whose dropSession closes this session's queue,
	// so the exit has to be queued before that happens.
	for {
		stream, payload, err := ReadFrame(conn)
		assert.NilError(t, err)
		if stream != StreamControl {
			continue
		}
		var c Control
		assert.NilError(t, json.Unmarshal(payload, &c))
		if c.Type == ControlHello {
			continue
		}
		assert.Equal(t, c.Type, ControlExit)
		return
	}
}

func TestBrokerSetStdinRacesWithClose(t *testing.T) {
	t.Parallel()

	// startBroker opens the container's stdin in the background, so SetStdin
	// can land at any moment, including after the container has exited. The
	// loser of that race has to be told, so that it closes its own descriptor
	// instead of leaking it.
	for range 200 {
		b := NewBroker(false, nil)
		stdin := &syncBuffer{}

		var wg sync.WaitGroup
		var took bool
		wg.Go(func() { took = b.SetStdin(stdin) })
		wg.Go(func() { b.Close(true) })
		wg.Wait()

		// Whoever ended up owning it closed it: SetStdin lost and the caller
		// closes, or SetStdin won and Close took it.
		if !took {
			stdin.Close()
		}
		assert.Equal(t, stdin.isClosed(), true)
	}
}

func TestBrokerCloseWithoutAnExitDoesNotAnnounceOne(t *testing.T) {
	t.Parallel()

	// The owner's stdio ending is not proof that the container exited: the same
	// thing happens when the logging process is shut down while the container
	// keeps running. A session told otherwise would wait for an exit code that
	// never arrives.
	b := NewBroker(true, nil)
	conn := connect(t, b)
	waitSessions(t, b, 1)

	b.Close(false)

	for {
		stream, payload, err := ReadFrame(conn)
		if err != nil {
			// EOF with no exit frame, which is what a broken session looks like.
			return
		}
		if stream != StreamControl {
			continue
		}
		var c Control
		assert.NilError(t, json.Unmarshal(payload, &c))
		assert.Assert(t, c.Type != ControlExit, "Close(false) announced an exit")
	}
}

func TestBrokerCloseDeliversTheExitPastAFullQueue(t *testing.T) {
	t.Parallel()

	// A session whose queue is exactly full has received all of the container's
	// output and is not behind. Pushing the exit through that same queue would
	// drop it, and the client would report a lost connection for a container
	// that finished normally.
	b := NewBroker(true, nil)
	conn := connect(t, b)
	waitSessions(t, b, 1)

	// Fill the queue without draining the connection.
	for range defaultQueueDepth {
		b.Write(StreamStdout, []byte("x"))
	}
	b.Close(true)

	for {
		stream, payload, err := ReadFrame(conn)
		assert.NilError(t, err)
		if stream != StreamControl {
			continue
		}
		var c Control
		assert.NilError(t, json.Unmarshal(payload, &c))
		if c.Type == ControlHello {
			continue
		}
		assert.Equal(t, c.Type, ControlExit)
		return
	}
}

func TestBrokerCloseAnnouncesExit(t *testing.T) {
	t.Parallel()

	b := NewBroker(true, nil)
	conn := connect(t, b)
	waitSessions(t, b, 1)

	b.Close(true)

	stream, payload, err := ReadFrame(conn)
	assert.NilError(t, err)
	assert.Equal(t, stream, StreamControl)

	var c Control
	assert.NilError(t, json.Unmarshal(payload, &c))
	assert.Equal(t, c.Type, ControlExit)

	// The broker closes the connection right after announcing the exit.
	_, _, err = ReadFrame(conn)
	assert.Assert(t, err != nil)
}

func TestBrokerCloseFlushesQueuedFrames(t *testing.T) {
	t.Parallel()

	// containerd calls os.Exit as soon as the logging function returns, so
	// whatever Close leaves queued is lost. The container's last chunk of
	// output has to reach the session before Close returns.
	b := NewBroker(true, nil)
	conn := connect(t, b)
	waitSessions(t, b, 1)

	type frame struct {
		stream  byte
		payload []byte
	}
	frames := make(chan frame, 8)
	go func() {
		for {
			stream, payload, err := ReadFrame(conn)
			if err != nil {
				close(frames)
				return
			}
			frames <- frame{stream, payload}
		}
	}()

	b.Write(StreamStdout, []byte("last words"))
	b.Close(true)

	var sawOutput, sawExit bool
	for range 2 {
		select {
		case f, ok := <-frames:
			if !ok {
				t.Fatal("the connection closed before the queued frames arrived")
			}
			if f.stream == StreamStdout && string(f.payload) == "last words" {
				sawOutput = true
			}
			if f.stream == StreamControl {
				sawExit = true
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out reading the frames Close should have flushed")
		}
	}
	assert.Assert(t, sawOutput, "the container's last output was dropped by Close")
	assert.Assert(t, sawExit, "the exit message was dropped by Close")
}

func TestBrokerCloseRacesWithDisconnectingSessions(t *testing.T) {
	t.Parallel()

	// A container exiting at the same moment as a session detaches must not
	// send on a channel that dropSession has just closed. That would panic, and
	// since the broker lives inside the logging process it would take logging
	// down with it. Run with -race.
	for range 50 {
		b := NewBroker(true, nil)
		conns := make([]net.Conn, 0, 4)
		for range 4 {
			conns = append(conns, connect(t, b))
		}
		waitSessions(t, b, 4)

		var wg sync.WaitGroup
		for _, conn := range conns {
			wg.Go(func() { conn.Close() })
		}
		wg.Go(func() { b.Close(true) })
		wg.Wait()
	}
}

func TestBrokerServeAcceptsSessions(t *testing.T) {
	t.Parallel()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NilError(t, err)
	t.Cleanup(func() { l.Close() })

	b := NewBroker(true, nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	served := make(chan error, 1)
	go func() { served <- b.Serve(ctx, l) }()

	conn, err := net.Dial("tcp", l.Addr().String())
	assert.NilError(t, err)
	t.Cleanup(func() { conn.Close() })

	stream, _, err := ReadFrame(conn)
	assert.NilError(t, err)
	assert.Equal(t, stream, StreamControl)

	cancel()
	select {
	case err := <-served:
		assert.NilError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after the context was cancelled")
	}
}
