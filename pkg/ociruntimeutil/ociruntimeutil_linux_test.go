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

package ociruntimeutil

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opencontainers/runtime-spec/specs-go/features"
	"gotest.tools/v3/assert"
)

// fakeRuntime creates a fake OCI runtime binary whose `features` subcommand
// prints featuresJSON, and appends a line to the log file on every execution.
func fakeRuntime(t *testing.T, featuresJSON string) (binary, execLog string) {
	t.Helper()
	dir := t.TempDir()
	binary = filepath.Join(dir, "fake-runtime")
	execLog = filepath.Join(dir, "exec.log")
	script := `#!/bin/sh
set -eu
echo executed >>` + execLog + `
if [ "${1:-}" != "features" ]; then
	echo >&2 "unknown command ${1:-}"
	exit 1
fi
cat <<'EOF'
` + featuresJSON + `
EOF
`
	assert.NilError(t, os.WriteFile(binary, []byte(script), 0o700))
	return binary, execLog
}

func countLines(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0
	}
	assert.NilError(t, err)
	n := 0
	for _, c := range b {
		if c == '\n' {
			n++
		}
	}
	return n
}

func TestFeatures(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	binary, execLog := fakeRuntime(t, `{"ociVersionMin": "1.0.0", "ociVersionMax": "1.2.0", "mountOptions": ["ro", "rro", "rbind"]}`)

	f, err := Features(binary)
	assert.NilError(t, err)
	assert.DeepEqual(t, []string{"ro", "rro", "rbind"}, f.MountOptions)
	assert.Equal(t, 1, countLines(t, execLog))

	// The second call must not execute the binary again (in-process cache).
	f, err = Features(binary)
	assert.NilError(t, err)
	assert.DeepEqual(t, []string{"ro", "rro", "rbind"}, f.MountOptions)
	assert.Equal(t, 1, countLines(t, execLog))

	// Drop the in-process cache: the XDG cache must be used, still without executing the binary.
	featuresCacheMu.Lock()
	featuresCache = make(map[string]*features.Features)
	featuresCacheMu.Unlock()
	f, err = Features(binary)
	assert.NilError(t, err)
	assert.DeepEqual(t, []string{"ro", "rro", "rbind"}, f.MountOptions)
	assert.Equal(t, 1, countLines(t, execLog))

	// Modifying the binary must invalidate the XDG cache entry
	// (the cache file is older than the binary now).
	// The mtime is set explicitly, as the timestamps of the cache file and the
	// binary might collide otherwise.
	future := time.Now().Add(time.Hour)
	assert.NilError(t, os.Chtimes(binary, future, future))
	featuresCacheMu.Lock()
	featuresCache = make(map[string]*features.Features)
	featuresCacheMu.Unlock()
	f, err = Features(binary)
	assert.NilError(t, err)
	assert.DeepEqual(t, []string{"ro", "rro", "rbind"}, f.MountOptions)
	assert.Equal(t, 2, countLines(t, execLog))
}

func TestFeaturesError(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	dir := t.TempDir()
	binary := filepath.Join(dir, "fake-runtime-no-features")
	assert.NilError(t, os.WriteFile(binary, []byte("#!/bin/sh\necho >&2 'unknown command'\nexit 1\n"), 0o700))
	_, err := Features(binary)
	assert.ErrorContains(t, err, "unknown command")
}
