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

package jsonfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type temporarilyFullWriter struct {
	buf    bytes.Buffer
	writes int
}

func (w *temporarilyFullWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == 1 {
		n, _ := w.buf.Write(p[:len(p)/2])
		return n, errors.New("disk full")
	}
	if w.writes == 2 {
		return 0, errors.New("disk full")
	}
	return w.buf.Write(p)
}

func TestEncodeDrainsAndRecoversAfterPartialWrite(t *testing.T) {
	stdout, stderr := make(chan string), make(chan string)
	w := &temporarilyFullWriter{}
	done := make(chan error, 1)
	go func() { done <- Encode(stdout, stderr, w) }()
	for _, line := range []string{"partial\n", "dropped\n", "recovered\n", "later\n"} {
		select {
		case stdout <- line:
		case <-time.After(time.Second):
			t.Fatal("output blocked after log write failure")
		}
	}
	close(stdout)
	close(stderr)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("encoder did not finish draining")
	}
	lines := strings.Split(strings.TrimSuffix(w.buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected partial plus two recovered records: %q", w.buf.String())
	}
	for i, want := range []string{"recovered\n", "later\n"} {
		var e Entry
		if err := json.Unmarshal([]byte(lines[i+1]), &e); err != nil {
			t.Fatal(err)
		}
		if e.Log != want || e.Stream != "stdout" {
			t.Fatalf("bad recovered record: %+v", e)
		}
	}
}
