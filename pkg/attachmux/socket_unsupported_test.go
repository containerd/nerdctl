//go:build !(linux || freebsd)

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
	"testing"

	"gotest.tools/v3/assert"
)

// On these platforms the stubs are the only transport code that runs, and
// callers branch on ErrUnsupported: run -it uses Probe to decide whether to
// hand the container's stdio to the logging process at all. A stub that
// returned nil would send it down a path with no broker behind it.
func TestTransportIsUnsupported(t *testing.T) {
	t.Parallel()

	assert.ErrorIs(t, Probe(t.TempDir()), ErrUnsupported)

	_, err := Listen("ignored")
	assert.ErrorIs(t, err, ErrUnsupported)

	_, err = Dial(context.Background(), "ignored")
	assert.ErrorIs(t, err, ErrUnsupported)

	// Removing a socket that was never created is not an error anywhere.
	assert.NilError(t, RemoveSocket("ignored"))
}
