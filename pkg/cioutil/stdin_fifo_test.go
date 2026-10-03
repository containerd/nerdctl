//go:build unix

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
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"gotest.tools/v3/assert"
)

func TestStdinFIFOPath(t *testing.T) {
	t.Parallel()

	got := StdinFIFOPath("/var/lib/nerdctl/1935db59", "default", "abc123")
	assert.Equal(t, got, filepath.Join("/var/lib/nerdctl/1935db59", "containers", "default", "abc123", "stdin.fifo"))
}

func TestCreateStdinFIFO(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "stdin.fifo")
	assert.NilError(t, CreateStdinFIFO(path))

	info, err := os.Stat(path)
	assert.NilError(t, err)
	assert.Assert(t, info.Mode()&os.ModeNamedPipe != 0, "%q is not a FIFO, mode %v", path, info.Mode())
	assert.Equal(t, info.Mode().Perm(), os.FileMode(0600))
}

func TestCreateStdinFIFOReplacesALeftover(t *testing.T) {
	t.Parallel()

	// A container that was killed leaves its FIFO behind; restarting it must
	// not fail with EEXIST.
	path := filepath.Join(t.TempDir(), "stdin.fifo")
	assert.NilError(t, CreateStdinFIFO(path))
	assert.NilError(t, CreateStdinFIFO(path))
}

func TestCreateStdinFIFOIsWriteOnlyBlockedWithoutAReader(t *testing.T) {
	t.Parallel()

	// The broker tells "this container has stdin" from "this FIFO is left over
	// from a previous run" by whether an O_WRONLY open succeeds. That only works
	// if the FIFO really is a FIFO.
	path := filepath.Join(t.TempDir(), "stdin.fifo")
	assert.NilError(t, CreateStdinFIFO(path))

	_, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
	assert.ErrorIs(t, err, syscall.ENXIO)
}
