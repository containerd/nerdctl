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
	"bytes"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestRunHelpDoesNotDuplicateDefaults(t *testing.T) {
	cmd := RunCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	assert.NilError(t, err)

	help := stdout.String()
	assert.Assert(t, strings.Contains(help, "Proxy received signals to the process (default true)"))
	assert.Assert(t, !strings.Contains(help, "Proxy received signals to the process (default true) (default true)"))
	assert.Assert(t, strings.Contains(help, "Tune container memory swappiness (0 to 100) (default -1)"))
	assert.Assert(t, !strings.Contains(help, "Tune container memory swappiness (0 to 100) (default -1) (default -1)"))
	assert.Assert(t, strings.Contains(help, "Allow running systemd in this container (default \"false\")"))
	assert.Assert(t, !strings.Contains(help, "Allow running systemd in this container (default: false) (default \"false\")"))
	assert.Assert(t, strings.Contains(help, "Time between running the check; 0 uses the image value or 30s when unset there too"))
	assert.Assert(t, strings.Contains(help, "Maximum time to allow one check to run; 0 uses the image value or 30s when unset there too"))
	assert.Assert(t, strings.Contains(help, "Consecutive failures needed to report unhealthy; 0 uses the image value or 3 when unset there too"))
}
