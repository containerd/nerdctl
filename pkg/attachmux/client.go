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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
)

// Session is a client's end of an attach connection.
type Session struct {
	conn  net.Conn
	hello Control

	// writeMu serialises writes to the connection.
	writeMu sync.Mutex

	// exitMu guards exited and closedByCaller.
	exitMu sync.Mutex
	exited bool
	// closedByCaller records that Close was called, so that Stream can tell a
	// deliberate teardown from the broker vanishing.
	closedByCaller bool

	// stopStdin is closed when Stream returns, so that the stdin pump stops
	// forwarding what it is about to read. It is only set up when Stream was
	// given a stdin.
	stopStdin chan struct{}
}

// NewSession completes the handshake on conn and returns the session. The
// caller keeps ownership of conn: closing the Session closes it.
func NewSession(conn net.Conn) (*Session, error) {
	stream, payload, err := ReadFrame(conn)
	if err != nil {
		return nil, fmt.Errorf("attachmux: failed to read the greeting: %w", err)
	}
	if stream != StreamControl {
		return nil, fmt.Errorf("attachmux: expected a control greeting, got stream %d", stream)
	}
	var hello Control
	if err := json.Unmarshal(payload, &hello); err != nil {
		return nil, fmt.Errorf("attachmux: failed to decode the greeting: %w", err)
	}
	if hello.Type != ControlHello {
		return nil, fmt.Errorf("attachmux: expected a hello, got %q", hello.Type)
	}
	if hello.Version != ProtocolVersion {
		return nil, fmt.Errorf("attachmux: unsupported protocol version %d, expected %d", hello.Version, ProtocolVersion)
	}
	return &Session{conn: conn, hello: hello}, nil
}

// TTY reports whether the container was created with a terminal, in which case
// it has a single output stream.
func (s *Session) TTY() bool { return s.hello.TTY }

// Exited reports whether Stream returned because the broker announced that the
// container is gone, as opposed to the user detaching or the context ending.
// The exit code itself comes from containerd.
func (s *Session) Exited() bool {
	s.exitMu.Lock()
	defer s.exitMu.Unlock()
	return s.exited
}

// Close closes the session's connection.
// Close tears the session down. A Stream running concurrently returns nil
// rather than an error: this is the caller ending the session, not the broker
// going away.
func (s *Session) wasClosedByCaller() bool {
	s.exitMu.Lock()
	defer s.exitMu.Unlock()
	return s.closedByCaller
}

func (s *Session) Close() error {
	s.exitMu.Lock()
	s.closedByCaller = true
	s.exitMu.Unlock()
	return s.conn.Close()
}

func (s *Session) writeFrame(frame []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.conn.Write(frame)
	return err
}

// Stream pumps the session until the broker announces the container's exit, the
// connection is closed, or ctx is done. Container output goes to stdout and
// stderr; when stderr is nil, the container's stderr is merged into stdout,
// which is what a TTY container needs.
//
// When stdin is non-nil, it is copied to the container in the background.
// Stream does not wait for that copy: a reader that is detachable returns an
// error when the user types the detach sequence, and the caller tears the
// session down.
func (s *Session) Stream(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) error {
	if stdin != nil {
		s.stopStdin = make(chan struct{})
		go s.pumpStdin(stdin)
	}

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			s.conn.Close()
		case <-done:
		}
	}()

	if stdin != nil {
		// pumpStdin blocks in stdin.Read and cannot be interrupted, so it is
		// left to end on its own. Tell it to stop forwarding once Stream is
		// done, otherwise the keystroke it is already waiting for is swallowed
		// from the terminal rather than reaching whatever runs next.
		defer close(s.stopStdin)
	}

	for {
		stream, payload, err := ReadFrame(s.conn)
		if err != nil {
			if ctx.Err() != nil || s.wasClosedByCaller() {
				// The caller ended this session: a detach, a cancelled command,
				// or an explicit Close. That is a normal outcome.
				return nil
			}
			// Anything else is the broker going away without saying the
			// container exited: the logging process died, or this session was
			// dropped for falling behind. Reporting nil here would look exactly
			// like a clean detach and the command would exit 0.
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return fmt.Errorf("attachmux: lost the connection to the container attach socket: %w", err)
			}
			return err
		}

		switch stream {
		case StreamStdout:
			if stdout == nil {
				continue
			}
			if _, err := stdout.Write(payload); err != nil {
				return err
			}
		case StreamStderr:
			w := stderr
			if w == nil {
				w = stdout
			}
			if w == nil {
				continue
			}
			if _, err := w.Write(payload); err != nil {
				return err
			}
		case StreamControl:
			var c Control
			if err := json.Unmarshal(payload, &c); err != nil {
				continue
			}
			if c.Type == ControlExit {
				s.exitMu.Lock()
				s.exited = true
				s.exitMu.Unlock()
				return nil
			}
		}
	}
}

func (s *Session) pumpStdin(stdin io.Reader) {
	buf := make([]byte, 32<<10)
	for {
		n, err := stdin.Read(buf)
		if n > 0 {
			select {
			case <-s.stopStdin:
				// Stream has returned; this session is over.
				return
			default:
			}
			frame, ferr := EncodeFrame(StreamStdin, buf[:n])
			if ferr != nil {
				return
			}
			if werr := s.writeFrame(frame); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}
