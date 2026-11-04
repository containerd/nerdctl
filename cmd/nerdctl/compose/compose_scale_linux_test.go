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

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestComposeScale(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		dockerComposeYAML := fmt.Sprintf(`
services:
  svc0:
    image: %s
    command: "sleep infinity"
`, testutil.CommonImage)

		composePath := data.Temp().Save(dockerComposeYAML, "compose.yaml")
		projectName := filepath.Base(filepath.Dir(composePath))
		data.Labels().Set("composeYAML", composePath)
		for _, n := range []string{"1", "2", "3"} {
			data.Labels().Set("container"+n, serviceparser.DefaultContainerName(projectName, "svc0", n))
		}

		helpers.Ensure("compose", "-f", composePath, "up", "-d")
		nerdtest.EnsureContainerStarted(helpers, data.Labels().Get("container1"))
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if data.Labels().Get("composeYAML") != "" {
			helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down", "-v")
		}
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "rejects an argument without replicas",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "scale", "svc0")
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			Description: "rejects an unknown service",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "scale", "unknown=2")
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			Description: "scales up",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "scale", "svc0=3")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						for _, n := range []string{"1", "2", "3"} {
							nerdtest.EnsureContainerStarted(helpers, data.Labels().Get("container"+n))
						}
					},
				}
			},
		},
		{
			Description: "scales down",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				// Scaling down must keep the remaining replica as it is, not recreate it.
				data.Labels().Set("container1ID", helpers.Capture("container", "inspect", "--format", "{{.ID}}", data.Labels().Get("container1")))
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "scale", "svc0=1")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						id := helpers.Capture("container", "inspect", "--format", "{{.ID}}", data.Labels().Get("container1"))
						assert.Equal(t, data.Labels().Get("container1ID"), id)
						helpers.Fail("container", "inspect", data.Labels().Get("container2"))
						helpers.Fail("container", "inspect", data.Labels().Get("container3"))
					},
				}
			},
		},
	}

	testCase.Run(t)
}
