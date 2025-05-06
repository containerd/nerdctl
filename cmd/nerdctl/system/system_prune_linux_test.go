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
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestSystemPrune(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.NoParallel = true

	testCase.SubTests = []*test.Case{
		{
			Description: "volume prune all success",
			// Private because of prune evidently
			Require: nerdtest.Private,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("network", "create", data.Identifier())
				helpers.Ensure("volume", "create", data.Identifier())
				anonIdentifier := helpers.Capture("volume", "create")
				helpers.Ensure("run", "-v", fmt.Sprintf("%s:/volume", data.Identifier()),
					"--net", data.Identifier(), "--name", data.Identifier(), testutil.CommonImage)

				data.Labels().Set("anonIdentifier", anonIdentifier)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("network", "rm", data.Identifier())
				helpers.Anyhow("volume", "rm", data.Identifier())
				helpers.Anyhow("volume", "rm", data.Labels().Get("anonIdentifier"))
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: test.Command("system", "prune", "-f", "--volumes", "--all"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, info string, t *testing.T) {
						volumes := helpers.Capture("volume", "ls")
						networks := helpers.Capture("network", "ls")
						images := helpers.Capture("images")
						containers := helpers.Capture("ps", "-a")
						assert.Assert(t, strings.Contains(volumes, data.Identifier()), volumes)
						assert.Assert(t, !strings.Contains(volumes, data.Labels().Get("anonIdentifier")), volumes)
						assert.Assert(t, !strings.Contains(containers, data.Identifier()), containers)
						assert.Assert(t, !strings.Contains(networks, data.Identifier()), networks)
						assert.Assert(t, !strings.Contains(images, testutil.CommonImage), images)
					},
				}
			},
		},
		{
			Description: "buildkit",
			// FIXME: using a dedicated namespace does not work with rootful (because of buildkitd)
			NoParallel: true,
			// buildkitd is not available with docker
			Require: require.All(nerdtest.Build, require.Not(nerdtest.Docker)),
			// FIXME: this test will happily say "green" even if the command actually fails to do its duty
			// if there is nothing in the build cache.
			// Ensure with setup here that we DO build something first
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("system", "prune", "-f", "--volumes", "--all")
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return nerdtest.BuildCtlCommand(helpers, "du")
			},
			Expected: test.Expects(0, nil, expect.Contains("Total:\t\t0B")),
		},
	}

	testCase.Run(t)
}
