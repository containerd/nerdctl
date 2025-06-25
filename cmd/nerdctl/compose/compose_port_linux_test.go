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
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestComposePort(t *testing.T) {
	base := testutil.NewBase(t)

	var dockerComposeYAML = fmt.Sprintf(`
services:
  svc0:
    image: %s
    command: "sleep infinity"
    ports:
    - "12345:10000"
    - "12346:10001/udp"
`, testutil.CommonImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").AssertOK()

	// `port` should work for given port and protocol
	base.ComposeCmd("-f", comp.YAMLFullPath(), "port", "svc0", "10000").AssertOutExactly("0.0.0.0:12345\n")
	base.ComposeCmd("-f", comp.YAMLFullPath(), "port", "--protocol", "udp", "svc0", "10001").AssertOutExactly("0.0.0.0:12346\n")
}

func TestComposePortFailure(t *testing.T) {
	base := testutil.NewBase(t)

	var dockerComposeYAML = fmt.Sprintf(`
services:
  svc0:
    image: %s
    command: "sleep infinity"
    ports:
    - "12345:10000"
    - "12346:10001/udp"
`, testutil.CommonImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").AssertOK()

	// `port` should fail if given port and protocol don't exist
	base.ComposeCmd("-f", comp.YAMLFullPath(), "port", "svc0", "9999").AssertFail()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "port", "--protocol", "udp", "svc0", "10000").AssertFail()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "port", "--protocol", "tcp", "svc0", "10001").AssertFail()
}

// TestComposeMultiplePorts tests whether it is possible to allocate a large
// number of ports. (https://github.com/containerd/nerdctl/issues/4027)
func TestComposeMultiplePorts(t *testing.T) {
	var dockerComposeYAML = fmt.Sprintf(`
services:
  svc0:
    image: %s
    command: "sleep infinity"
    ports:
    - '32000-32060:32000-32060'
`, testutil.AlpineImage)

	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		compYamlPath := data.Temp().Save(dockerComposeYAML, "compose.yaml")
		data.Labels().Set("composeYaml", compYamlPath)

		helpers.Ensure("compose", "-f", compYamlPath, "up", "-d")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down", "-v")
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "Issue #4027 - Allocate a large number of ports.",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYaml"), "port", "svc0", "32000")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("0.0.0.0:32000")),
		},
	}

	testCase.Run(t)
}
