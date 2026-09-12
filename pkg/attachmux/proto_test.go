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
	"encoding/json"
	"io"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestEncodeFrameRoundTrip(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		stream  byte
		payload []byte
	}{
		{"stdout", StreamStdout, []byte("hello")},
		{"stderr", StreamStderr, []byte("oops")},
		{"stdin", StreamStdin, []byte{0x16, 0x11}},
		{"empty", StreamStdout, []byte{}},
		{"binary", StreamStdout, []byte{0x00, 0xff, 0x1b, 0x5b, 0x41}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			frame, err := EncodeFrame(tc.stream, tc.payload)
			assert.NilError(t, err)

			stream, payload, err := ReadFrame(bytes.NewReader(frame))
			assert.NilError(t, err)
			assert.Equal(t, stream, tc.stream)
			assert.DeepEqual(t, payload, tc.payload)
		})
	}
}

func TestReadFrameConsecutive(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	for _, s := range []string{"one", "two", "three"} {
		frame, err := EncodeFrame(StreamStdout, []byte(s))
		assert.NilError(t, err)
		buf.Write(frame)
	}

	for _, want := range []string{"one", "two", "three"} {
		stream, payload, err := ReadFrame(&buf)
		assert.NilError(t, err)
		assert.Equal(t, stream, StreamStdout)
		assert.Equal(t, string(payload), want)
	}

	_, _, err := ReadFrame(&buf)
	assert.ErrorIs(t, err, io.EOF)
}

func TestEncodeFrameRejectsOversizedPayload(t *testing.T) {
	t.Parallel()

	_, err := EncodeFrame(StreamStdout, make([]byte, maxPayload+1))
	assert.ErrorIs(t, err, ErrPayloadTooLarge)
}

func TestReadFrameRejectsOversizedHeader(t *testing.T) {
	t.Parallel()

	// A header announcing more than maxPayload must be refused before the
	// reader allocates for it.
	hdr := []byte{StreamStdout, 0, 0, 0, 0xff, 0xff, 0xff, 0xff}
	_, _, err := ReadFrame(bytes.NewReader(hdr))
	assert.ErrorIs(t, err, ErrPayloadTooLarge)
}

func TestReadFrameTruncated(t *testing.T) {
	t.Parallel()

	frame, err := EncodeFrame(StreamStdout, []byte("hello"))
	assert.NilError(t, err)

	_, _, err = ReadFrame(bytes.NewReader(frame[:len(frame)-2]))
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
}

func TestEncodeControl(t *testing.T) {
	t.Parallel()

	frame, err := EncodeControl(Control{Type: ControlHello, Version: ProtocolVersion, TTY: true})
	assert.NilError(t, err)

	stream, payload, err := ReadFrame(bytes.NewReader(frame))
	assert.NilError(t, err)
	assert.Equal(t, stream, StreamControl)

	var c Control
	assert.NilError(t, json.Unmarshal(payload, &c))
	assert.Equal(t, c.Type, ControlHello)
	assert.Equal(t, c.Version, 1)
	assert.Equal(t, c.TTY, true)
}

func TestEncodeControlExitCarriesNoCode(t *testing.T) {
	t.Parallel()

	// The exit message only says the container is gone. The exit code always
	// comes from containerd, so that there is one source of truth for it.
	frame, err := EncodeControl(Control{Type: ControlExit})
	assert.NilError(t, err)

	_, payload, err := ReadFrame(bytes.NewReader(frame))
	assert.NilError(t, err)
	assert.Equal(t, string(payload), `{"type":"exit"}`)
}

func TestReadFrameFromStream(t *testing.T) {
	t.Parallel()

	// A reader that hands out one byte at a time still yields a whole frame.
	frame, err := EncodeFrame(StreamStdout, []byte("drip"))
	assert.NilError(t, err)

	stream, payload, err := ReadFrame(iotest(strings.NewReader(string(frame))))
	assert.NilError(t, err)
	assert.Equal(t, stream, StreamStdout)
	assert.Equal(t, string(payload), "drip")
}

// iotest wraps r so that each Read returns at most one byte.
func iotest(r io.Reader) io.Reader { return &oneByteReader{r: r} }

type oneByteReader struct{ r io.Reader }

func (o *oneByteReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return o.r.Read(p[:1])
}
