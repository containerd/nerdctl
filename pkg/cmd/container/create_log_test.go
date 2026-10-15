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

package container

import (
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/logging"
)

// generateLogConfig writes into the container's state directory, which the
// caller has already created by the time it runs.
func stateDirFor(t *testing.T, ns, id string) string {
	t.Helper()
	dataStore := t.TempDir()
	assert.NilError(t, os.MkdirAll(filepath.Join(dataStore, "containers", ns, id), 0700))
	return dataStore
}

// The attach broker lives in the logging process, which never sees the CLI's
// flags. Without the choice written into the log config, switching the broker
// off would still leave a detached container serving attach sessions.
func TestGenerateLogConfigRecordsDisableAttachBroker(t *testing.T) {
	const ns, id = "testns", "testid"

	for _, disabled := range []bool{false, true} {
		dataStore := stateDirFor(t, ns, id)
		logConfig, err := generateLogConfig(dataStore, id, "json-file", nil, ns, "", disabled, false)
		assert.NilError(t, err)
		assert.Equal(t, logConfig.DisableAttachBroker, disabled)

		// What the logging process actually reads.
		loaded, err := logging.LoadLogConfig(dataStore, ns, id)
		assert.NilError(t, err)
		assert.Equal(t, loaded.DisableAttachBroker, disabled)
	}
}

// "none" is a registered no-op driver, not an absence of logging. By default it
// goes through the internal logging process, which is what owns the container's
// stdio; with the broker off there is nothing to own it, so it short-circuits
// again and the container runs with no logging process at all.
func TestGenerateLogConfigNoneFollowsTheBroker(t *testing.T) {
	const ns, id = "testns", "testid"

	logConfig, err := generateLogConfig(stateDirFor(t, ns, id), id, "none", nil, ns, "", false, false)
	assert.NilError(t, err)
	assert.Equal(t, logConfig.Driver, "none")
	assert.Assert(t, logConfig.LogURI != "none", "expected the internal log URI, got %q", logConfig.LogURI)

	logConfig, err = generateLogConfig(stateDirFor(t, ns, id), id, "none", nil, ns, "", true, false)
	assert.NilError(t, err)
	assert.Equal(t, logConfig.LogURI, "none")
}
