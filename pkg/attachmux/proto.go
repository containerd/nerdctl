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

// Package attachmux multiplexes a container's stdio between the process that
// owns it and any number of attached CLI sessions.
//
// A container's stdio FIFOs cannot be shared: a FIFO has a single queue, so two
// readers on the stdout FIFO each receive a random subset of the container's
// output. Instead, one process owns the FIFOs and every session talks to it
// over a socket, framed with the protocol in this file.
package attachmux

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Stream identifiers carried in a frame header.
const (
	StreamStdin   byte = 0
	StreamStdout  byte = 1
	StreamStderr  byte = 2
	StreamControl byte = 3
)

// Types of a StreamControl message.
const (
	// ControlHello is sent by the broker to a session as soon as it connects.
	ControlHello = "hello"
	// ControlExit is sent by the broker when the container has exited. It
	// carries no exit code: a session reads that from containerd, so that there
	// is a single source of truth for it.
	ControlExit = "exit"
)

// ProtocolVersion is announced in the hello message so that a future change to
// the framing can be detected by an older client.
const ProtocolVersion = 1

const (
	headerSize = 8
	// maxPayload bounds a single frame so that a corrupt or hostile peer cannot
	// make the reader allocate without limit. Container output is read in 32 KiB
	// chunks, so this is never hit in practice.
	maxPayload = 1 << 20
)

// ErrPayloadTooLarge is returned for a frame whose payload exceeds maxPayload.
var ErrPayloadTooLarge = errors.New("attachmux: frame payload too large")

// Control is the JSON body of a StreamControl frame.
type Control struct {
	Type    string `json:"type"`
	Version int    `json:"version,omitempty"`
	TTY     bool   `json:"tty,omitempty"`
}

// EncodeFrame returns the wire representation of a single frame: one stream
// byte, three reserved zero bytes, a big-endian uint32 payload length, then the
// payload.
func EncodeFrame(stream byte, payload []byte) ([]byte, error) {
	if len(payload) > maxPayload {
		return nil, fmt.Errorf("%w: %d bytes", ErrPayloadTooLarge, len(payload))
	}
	buf := make([]byte, headerSize+len(payload))
	buf[0] = stream
	binary.BigEndian.PutUint32(buf[4:headerSize], uint32(len(payload)))
	copy(buf[headerSize:], payload)
	return buf, nil
}

// EncodeControl returns a StreamControl frame carrying c.
func EncodeControl(c Control) ([]byte, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	return EncodeFrame(StreamControl, b)
}

// ReadFrame reads exactly one frame. The returned payload is a fresh slice
// owned by the caller.
func ReadFrame(r io.Reader) (stream byte, payload []byte, err error) {
	var hdr [headerSize]byte
	if _, err = io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(hdr[4:headerSize])
	if n > maxPayload {
		return 0, nil, fmt.Errorf("%w: %d bytes", ErrPayloadTooLarge, n)
	}
	payload = make([]byte, n)
	if _, err = io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return hdr[0], payload, nil
}
