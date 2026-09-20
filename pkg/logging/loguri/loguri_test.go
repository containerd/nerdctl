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

package loguri

import (
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/containerd/v2/pkg/cio"
)

func TestIsInternal(t *testing.T) {
	self, err := os.Executable()
	assert.NilError(t, err)
	self, err = filepath.EvalSymlinks(self)
	assert.NilError(t, err)

	mine, err := cio.LogURIGenerator("binary", self, map[string]string{MagicArgv1: "/var/lib/nerdctl/1935db59"})
	assert.NilError(t, err)

	other, err := cio.LogURIGenerator("binary", os.TempDir(), map[string]string{MagicArgv1: "/var/lib/nerdctl/1935db59"})
	assert.NilError(t, err)

	for _, tc := range []struct {
		name string
		uri  string
		want bool
	}{
		{"this binary", mine.String(), true},
		// A URI written by an older nerdctl still installed elsewhere: it
		// parses, but spawning it would run a binary with no broker.
		{"another binary", other.String(), false},
		// What a nerdctl predating the broker wrote for --log-driver none.
		{"none", "none", false},
		{"empty", "", false},
		{"custom binary", "binary:///usr/local/bin/mylogger?foo=bar", false},
		{"file scheme", "file:///var/log/container.log", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, IsInternal(tc.uri), tc.want)
		})
	}
}

func TestDataStore(t *testing.T) {
	// LogURIGenerator rejects a path that is not absolute for the platform, and
	// "/usr/local/bin/nerdctl" is not one on windows.
	self, err := os.Executable()
	assert.NilError(t, err)

	mine, err := cio.LogURIGenerator("binary", self, map[string]string{MagicArgv1: "/var/lib/nerdctl/1935db59"})
	assert.NilError(t, err)

	// The broker derives the attach socket and the stdin FIFO from this exact
	// string, so a client has to use it verbatim rather than resolving the data
	// store again.
	assert.Equal(t, DataStore(mine.String()), "/var/lib/nerdctl/1935db59")
	assert.Equal(t, DataStore("none"), "")
	assert.Equal(t, DataStore(""), "")
	assert.Equal(t, DataStore("binary:///usr/local/bin/mylogger?foo=bar"), "")
}
