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

package compose

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

// setUpComposeWaitProject starts a project with a service that exits with
// code 3 after a few seconds, and a service that keeps running.
func setUpComposeWaitProject(data test.Data, helpers test.Helpers) {
	dockerComposeYAML := fmt.Sprintf(`
services:
  exiting:
    image: %s
    command: ["sh", "-c", "sleep 5; exit 3"]
  running:
    image: %s
    command: "sleep infinity"
`, testutil.CommonImage, testutil.CommonImage)

	composePath := data.Temp().Save(dockerComposeYAML, "compose.yaml")
	projectName := filepath.Base(filepath.Dir(composePath))
	data.Labels().Set("composeYAML", composePath)
	data.Labels().Set("runningContainer", serviceparser.DefaultContainerName(projectName, "running", "1"))

	helpers.Ensure("compose", "-f", composePath, "up", "-d")
	nerdtest.EnsureContainerStarted(helpers, data.Labels().Get("runningContainer"))
}

func cleanUpComposeWaitProject(data test.Data, helpers test.Helpers) {
	if data.Labels().Get("composeYAML") != "" {
		helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down", "-v")
	}
}

func TestComposeWait(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.SubTests = []*test.Case{
		{
			Description: "requires a service",
			Setup:       setUpComposeWaitProject,
			Cleanup:     cleanUpComposeWaitProject,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "wait")
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			Description: "exits with the exit code of the container",
			Setup:       setUpComposeWaitProject,
			Cleanup:     cleanUpComposeWaitProject,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "wait", "exiting")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 3,
					Output: func(stdout string, t tig.T) {
						expect.Contains("exited with status code 3")(stdout, t)
						// the project is kept without --down-project
						helpers.Ensure("container", "inspect", data.Labels().Get("runningContainer"))
					},
				}
			},
		},
		{
			Description: "drops the project with --down-project",
			Setup:       setUpComposeWaitProject,
			Cleanup:     cleanUpComposeWaitProject,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "wait", "--down-project", "exiting")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 3,
					Output: func(stdout string, t tig.T) {
						helpers.Fail("container", "inspect", data.Labels().Get("runningContainer"))
					},
				}
			},
		},
	}

	testCase.Run(t)
}
