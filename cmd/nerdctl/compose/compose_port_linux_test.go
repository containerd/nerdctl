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
	"strconv"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/portlock"
)

func TestComposePort(t *testing.T) {
	const portCount = 2

	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		for i := 0; i < portCount; i++ {
			port, err := portlock.Acquire(0)
			if err != nil {
				helpers.T().Log(fmt.Sprintf("Failed to acquire port: %v", err))
				helpers.T().FailNow()
			}
			data.Labels().Set(fmt.Sprintf("hostPort%d", i), strconv.Itoa(port))
		}

		dockerComposeYAML := fmt.Sprintf(`
services:
  svc0:
    image: %s
    command: "sleep infinity"
    ports:
    - "%s:10000"
    - "%s:10001/udp"
`, testutil.CommonImage, data.Labels().Get("hostPort0"), data.Labels().Get("hostPort1"))

		compYamlPath := data.Temp().Save(dockerComposeYAML, "compose.yaml")
		data.Labels().Set("composeYaml", compYamlPath)
		projectName := filepath.Base(filepath.Dir(compYamlPath))
		t.Logf("projectName=%q", projectName)

		helpers.Ensure("compose", "-f", compYamlPath, "up", "-d")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down", "-v")
		for i := 0; i < portCount; i++ {
			port, _ := strconv.Atoi(data.Labels().Get(fmt.Sprintf("hostPort%d", i)))
			_ = portlock.Release(port)
		}
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "port should return host port for TCP",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYaml"), "port", "svc0", "10000")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output:   expect.Equals(fmt.Sprintf("0.0.0.0:%s\n", data.Labels().Get("hostPort0"))),
				}
			},
		},
		{
			Description: "port should return host port for UDP",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYaml"), "port", "--protocol", "udp", "svc0", "10001")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output:   expect.Equals(fmt.Sprintf("0.0.0.0:%s\n", data.Labels().Get("hostPort1"))),
				}
			},
		},
	}

	testCase.Run(t)
}

func TestComposePortFailure(t *testing.T) {
	const portCount = 2

	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		for i := 0; i < portCount; i++ {
			port, err := portlock.Acquire(0)
			if err != nil {
				helpers.T().Log(fmt.Sprintf("Failed to acquire port: %v", err))
				helpers.T().FailNow()
			}
			data.Labels().Set(fmt.Sprintf("hostPort%d", i), strconv.Itoa(port))
		}

		dockerComposeYAML := fmt.Sprintf(`
services:
  svc0:
    image: %s
    command: "sleep infinity"
    ports:
    - "%s:10000"
    - "%s:10001/udp"
`, testutil.CommonImage, data.Labels().Get("hostPort0"), data.Labels().Get("hostPort1"))

		compYamlPath := data.Temp().Save(dockerComposeYAML, "compose.yaml")
		data.Labels().Set("composeYaml", compYamlPath)
		projectName := filepath.Base(filepath.Dir(compYamlPath))
		t.Logf("projectName=%q", projectName)

		helpers.Ensure("compose", "-f", compYamlPath, "up", "-d")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down", "-v")
		for i := 0; i < portCount; i++ {
			port, _ := strconv.Atoi(data.Labels().Get(fmt.Sprintf("hostPort%d", i)))
			_ = portlock.Release(port)
		}
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "port should fail for non-existent port",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYaml"), "port", "svc0", "9999")
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			Description: "port should fail for wrong protocol (UDP on TCP port)",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYaml"), "port", "--protocol", "udp", "svc0", "10000")
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			Description: "port should fail for wrong protocol (TCP on UDP port)",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYaml"), "port", "--protocol", "tcp", "svc0", "10001")
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
	}

	testCase.Run(t)
}

// TestComposeMultiplePorts tests whether it is possible to allocate a large
// number of ports. (https://github.com/containerd/nerdctl/issues/4027)
func TestComposeMultiplePorts(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.NoParallel = true

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		dockerComposeYAML := fmt.Sprintf(`
services:
  svc0:
    image: %s
    command: "sleep infinity"
    ports:
    - '32000-32060:32000-32060'
`, testutil.AlpineImage)

		compYamlPath := data.Temp().Save(dockerComposeYAML, "compose.yaml")
		data.Labels().Set("composeYaml", compYamlPath)
		projectName := filepath.Base(filepath.Dir(compYamlPath))
		t.Logf("projectName=%q", projectName)

		helpers.Ensure("compose", "-f", compYamlPath, "up", "-d")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down", "-v")
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "Issue #4027 - Allocate a large number of ports.",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYaml"), "port", "svc0", "32000")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("0.0.0.0:32000")),
		},
	}

	testCase.Run(t)
}
