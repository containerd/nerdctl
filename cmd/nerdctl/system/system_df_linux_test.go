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

package system

import (
	"fmt"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

// TestSystemDfVolumes covers the Local Volumes row, which the rest of TestSystemDf cannot: a volume
// is only counted once a container mounts it, and the target of a mount is written differently on
// each platform.
func TestSystemDfVolumes(t *testing.T) {
	testCase := nerdtest.Setup()

	// The counts are only meaningful when nothing else is running against the same namespace.
	testCase.NoParallel = true

	testCase.SubTests = []*test.Case{
		{
			Description: "mounted volume is active",
			Require:     nerdtest.Private,
			Setup: func(data test.Data, helpers test.Helpers) {
				data.Labels().Set(baselineLabel, helpers.Capture("system", "df"))
				helpers.Ensure("volume", "create", data.Identifier())
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"-v", fmt.Sprintf("%s:/volume", data.Identifier()),
					testutil.CommonImage, "sleep", nerdtest.Infinity)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
				helpers.Anyhow("volume", "rm", "-f", data.Identifier())
			},
			Command: test.Command("system", "df"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, t tig.T) {
						// The volume is created by this test and the container it runs mounts it,
						// so both counts went up by it.
						dfGrewBy(t, data, stdout, "Local Volumes", totalColumn, 1)
						dfGrewBy(t, data, stdout, "Local Volumes", activeColumn, 1)
					},
				}
			},
		},
	}

	testCase.Run(t)
}
