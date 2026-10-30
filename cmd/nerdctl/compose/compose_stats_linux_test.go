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
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestComposeStats(t *testing.T) {
	var dockerComposeYAML = fmt.Sprintf(`
services:
  svc0:
    image: %s
    command: "sleep infinity"
  svc1:
    image: %s
    command: "sleep infinity"
`, testutil.CommonImage, testutil.CommonImage)

	testCase := nerdtest.Setup()

	testCase.Require = require.All(nerdtest.CgroupsAccessible)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Temp().Save(dockerComposeYAML, "compose.yaml")
		data.Labels().Set("yamlPath", data.Temp().Path("compose.yaml"))
		data.Labels().Set("project", data.Identifier())
		helpers.Ensure("compose", "-f", data.Temp().Path("compose.yaml"), "-p", data.Identifier(), "up", "-d")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "-p", data.Identifier(), "down")
	}

	compose := func(data test.Data, helpers test.Helpers, args ...string) test.TestableCommand {
		return helpers.Command(append([]string{"compose", "-f", data.Labels().Get("yamlPath"), "-p", data.Labels().Get("project")}, args...)...)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "all services",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return compose(data, helpers, "stats", "--no-stream", "--no-trunc", "--format", "{{.Name}}")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.All(
						expect.Contains(data.Labels().Get("project")+"-svc0-1"),
						expect.Contains(data.Labels().Get("project")+"-svc1-1"),
					),
				}
			},
		},
		{
			Description: "one service",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return compose(data, helpers, "stats", "--no-stream", "--no-trunc", "--format", "{{.Name}}", "svc0")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.All(
						expect.Contains(data.Labels().Get("project")+"-svc0-1"),
						expect.DoesNotContain(data.Labels().Get("project")+"-svc1-1"),
					),
				}
			},
		},
		{
			Description: "stopped service is skipped",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("compose", "-f", data.Labels().Get("yamlPath"), "-p", data.Labels().Get("project"), "stop", "svc1")
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return compose(data, helpers, "stats", "--no-stream", "--no-trunc", "--format", "{{.Name}}", "svc1")
			},
			Expected: test.Expects(0, nil, expect.Equals("")),
		},
	}

	testCase.Run(t)
}
