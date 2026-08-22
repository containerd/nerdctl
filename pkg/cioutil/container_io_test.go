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

package cioutil

import (
	"errors"
	"os"
	"testing"
)

type failingWriter struct {
	calls int
}

func (f *failingWriter) Write(p []byte) (int, error) {
	f.calls++
	return 0, errors.New("broken pipe")
}

// TestBestEffortWriter verifies that a failing logger pipe never errors the
// stdio tee: the first failed write disables the writer and every write still
// reports full success, so the copier draining the container's stdio keeps
// running. Regression test for
// https://github.com/containerd/nerdctl/issues/5137
func TestBestEffortWriter(t *testing.T) {
	fw := &failingWriter{}
	b := &bestEffortWriter{w: fw}

	for i := 0; i < 3; i++ {
		n, err := b.Write([]byte("data"))
		if err != nil {
			t.Fatalf("write %d: best-effort writer must not return an error, got %v", i, err)
		}
		if n != 4 {
			t.Fatalf("write %d: expected n=4, got %d", i, n)
		}
	}
	if fw.calls != 1 {
		t.Fatalf("expected the underlying writer to be abandoned after the first failure, got %d calls", fw.calls)
	}
}

// TestBestEffortWriterClosedPipe exercises the real failure mode: writing to
// an os.Pipe whose read end is closed (EPIPE), as happens when the logging
// binary dies after our copies of its read ends were closed.
func TestBestEffortWriterClosedPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	defer w.Close()

	b := &bestEffortWriter{w: w}
	for i := 0; i < 2; i++ {
		if n, err := b.Write([]byte("data")); err != nil || n != 4 {
			t.Fatalf("write %d: expected (4, nil), got (%d, %v)", i, n, err)
		}
	}
	if !b.dead {
		t.Fatal("expected the writer to be marked dead after EPIPE")
	}
}
