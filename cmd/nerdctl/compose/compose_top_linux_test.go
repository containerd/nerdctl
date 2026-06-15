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

func TestComposeTop(t *testing.T) {
	var dockerComposeYAML = fmt.Sprintf(`
services:
  svc0:
    image: %s
    command: "sleep infinity"
  svc1:
    image: %s
`, testutil.CommonImage, testutil.NginxAlpineImage)

	testCase := nerdtest.Setup()

	testCase.Require = require.All(nerdtest.CgroupsAccessible)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Temp().Save(dockerComposeYAML, "compose.yaml")
		helpers.Ensure("compose", "-f", data.Temp().Path("compose.yaml"), "up", "-d")
		data.Labels().Set("yamlPath", data.Temp().Path("compose.yaml"))
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down")
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "svc0 contains sleep infinity",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("yamlPath"), "top", "svc0")
			},
			Expected: test.Expects(0, nil, expect.Contains("sleep infinity")),
		},
		{
			Description: "svc1 contains sleep nginx",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("yamlPath"), "top", "svc1")
			},
			Expected: test.Expects(0, nil, expect.Contains("nginx")),
		},
	}

	testCase.Run(t)
}
