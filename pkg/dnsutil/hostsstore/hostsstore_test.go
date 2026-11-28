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

package hostsstore

import (
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

func TestReleaseIsIdempotent(t *testing.T) {
	dataStore := t.TempDir()
	hs, err := New(dataStore, "default")
	assert.NilError(t, err)

	const id = "0123456789abcdef"
	_, err = hs.AllocHostsFile(id, []byte{})
	assert.NilError(t, err)
	assert.NilError(t, hs.Acquire(Meta{ID: id, Name: "foo"}))

	// `nerdctl stop` releases first, then the Poststop hook releases again on `start` or `rm`.
	assert.NilError(t, hs.Release(id))
	assert.NilError(t, hs.Release(id))

	// The hosts file must survive Release, as it is needed to restart the container.
	_, err = os.Stat(filepath.Join(dataStore, hostsDirBasename, "default", id, hostsFile))
	assert.NilError(t, err)
	_, err = os.Stat(filepath.Join(dataStore, hostsDirBasename, "default", id, metaJSON))
	assert.Assert(t, os.IsNotExist(err))
}

func TestReleaseUnknownContainer(t *testing.T) {
	hs, err := New(t.TempDir(), "default")
	assert.NilError(t, err)
	assert.NilError(t, hs.Release("unknown"))
}
