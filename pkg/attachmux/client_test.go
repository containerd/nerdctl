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
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

// lockedBuffer is an io.Writer safe for concurrent use.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// dropAllSessions disconnects every session without announcing an exit, which
// is what a broker whose process died looks like from the client side.
func dropAllSessions(b *Broker) {
	b.mu.Lock()
	sessions := make([]*session, 0, len(b.sessions))
	for s := range b.sessions {
		sessions = append(sessions, s)
	}
	b.sessions = map[*session]struct{}{}
	b.mu.Unlock()

	for _, s := range sessions {
		s.close()
	}
}

// pair returns a broker and a session connected to it over an in-memory pipe.
func pair(t *testing.T, tty bool, stdin io.WriteCloser) (*Broker, *Session) {
	t.Helper()

	b := NewBroker(tty, stdin)
	mine, theirs := net.Pipe()
	t.Cleanup(func() { mine.Close() })

	go b.addSession(theirs)

	s, err := NewSession(mine)
	assert.NilError(t, err)
	return b, s
}

func TestSessionHandshakeReportsTTY(t *testing.T) {
	t.Parallel()

	_, s := pair(t, true, nil)
	assert.Equal(t, s.TTY(), true)
}

func TestSessionStreamsOutput(t *testing.T) {
	t.Parallel()

	b, s := pair(t, false, nil)
	waitSessions(t, b, 1)

	stdout := &lockedBuffer{}
	stderr := &lockedBuffer{}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	done := make(chan error, 1)
	go func() { done <- s.Stream(ctx, nil, stdout, stderr) }()

	b.Write(StreamStdout, []byte("to stdout"))
	b.Write(StreamStderr, []byte("to stderr"))

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if stdout.String() == "to stdout" && stderr.String() == "to stderr" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	assert.Equal(t, stdout.String(), "to stdout")
	assert.Equal(t, stderr.String(), "to stderr")

	b.Close(true)
	select {
	case err := <-done:
		assert.NilError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Stream did not return after the broker announced the exit")
	}

	assert.Equal(t, s.Exited(), true)
}

func TestSessionExitedIsFalseAfterADetach(t *testing.T) {
	t.Parallel()

	// Detaching closes the connection without an exit message. The caller uses
	// this to tell "the user detached" from "the container is gone".
	b, s := pair(t, true, nil)
	waitSessions(t, b, 1)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Stream(ctx, nil, io.Discard, nil) }()

	cancel()
	select {
	case err := <-done:
		assert.NilError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Stream did not return after the context was cancelled")
	}

	assert.Equal(t, s.Exited(), false)
}

func TestSessionMergesStderrIntoStdoutWhenNoStderrWriter(t *testing.T) {
	t.Parallel()

	// With a TTY there is a single stream, and callers pass a nil stderr.
	b, s := pair(t, true, nil)
	waitSessions(t, b, 1)

	stdout := &lockedBuffer{}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go func() { _ = s.Stream(ctx, nil, stdout, nil) }()

	b.Write(StreamStderr, []byte("merged"))

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if stdout.String() == "merged" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("stderr was not merged into stdout, got %q", stdout.String())
}

func TestSessionSendsStdin(t *testing.T) {
	t.Parallel()

	stdin := &syncBuffer{}
	b, s := pair(t, false, stdin)
	waitSessions(t, b, 1)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go func() { _ = s.Stream(ctx, strings.NewReader("typed input\n"), io.Discard, nil) }()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if stdin.String() == "typed input\n" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("stdin did not reach the broker, got %q", stdin.String())
}

func TestSessionStreamReportsAnUnexpectedDisconnect(t *testing.T) {
	t.Parallel()

	// The broker dying is not the same as the user detaching. If it were
	// reported as a clean end of session, the command would exit 0 and the user
	// would never learn that the container's output stopped being delivered.
	b, s := pair(t, true, nil)
	waitSessions(t, b, 1)

	ctx := context.Background()
	done := make(chan error, 1)
	go func() { done <- s.Stream(ctx, nil, io.Discard, nil) }()

	// Drop the session the way a dead broker would: close the connection
	// without sending an exit message.
	dropAllSessions(b)

	select {
	case err := <-done:
		assert.Assert(t, err != nil, "an unexpected disconnect was reported as a clean detach")
		assert.Equal(t, s.Exited(), false)
	case <-time.After(5 * time.Second):
		t.Fatal("Stream did not return after the broker went away")
	}
}

func TestSessionStreamReturnsWhenContextIsCancelled(t *testing.T) {
	t.Parallel()

	b, s := pair(t, true, nil)
	waitSessions(t, b, 1)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Stream(ctx, nil, io.Discard, nil) }()

	cancel()
	select {
	case err := <-done:
		assert.NilError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Stream did not return after the context was cancelled")
	}
}

func TestSessionStreamTreatsAnExplicitCloseAsClean(t *testing.T) {
	t.Parallel()

	// Stream's own doc says the caller tears the session down, and Close is the
	// obvious way. Reporting that as a lost connection would be the same false
	// positive the unexpected-disconnect branch exists to avoid.
	b, session := pair(t, true, nil)
	_ = b

	streamed := make(chan error, 1)
	go func() {
		streamed <- session.Stream(context.Background(), nil, io.Discard, nil)
	}()

	time.Sleep(100 * time.Millisecond)
	assert.NilError(t, session.Close())

	select {
	case err := <-streamed:
		assert.NilError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Stream did not return after Close")
	}
}
