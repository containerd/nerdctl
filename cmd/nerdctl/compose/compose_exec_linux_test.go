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
	"net"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestComposeExec(t *testing.T) {
	dockerComposeYAML := fmt.Sprintf(`
version: '3.1'

services:
  svc0:
    image: %s
    command: "sleep infinity"
  svc1:
    image: %s
    command: "sleep infinity"
`, testutil.CommonImage, testutil.CommonImage)

	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		yamlPath := data.Temp().Save(dockerComposeYAML, "compose.yaml")
		data.Labels().Set("YAMLPath", yamlPath)
		helpers.Ensure("compose", "-f", yamlPath, "up", "-d", "svc0")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down", "-v")
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "exec no tty",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command(
					"compose",
					"-f",
					data.Labels().Get("YAMLPath"),
					"exec",
					"-i=false",
					"--no-TTY",
					"svc0",
					"echo",
					"success",
				)
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("success\n")),
		},
		{
			Description: "exec no tty with workdir",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command(
					"compose",
					"-f",
					data.Labels().Get("YAMLPath"),
					"exec",
					"-i=false",
					"--no-TTY",
					"--workdir",
					"/tmp",
					"svc0",
					"pwd",
				)
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("/tmp\n")),
		},
		{
			Description: "cannot exec on non-running service",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("YAMLPath"), "exec", "svc1", "echo", "success")
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			Description: "with env",
			Env: map[string]string{
				"CORGE":  "corge-value-in-host",
				"GARPLY": "garply-value-in-host",
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command(
					"compose",
					"-f",
					data.Labels().Get("YAMLPath"),
					"exec",
					"-i=false",
					"--no-TTY",
					"--env", "FOO=foo1,foo2",
					"--env", "BAR=bar1 bar2",
					"--env", "BAZ=",
					"--env", "QUX", // not exported in OS
					"--env", "QUUX=quux1",
					"--env", "QUUX=quux2",
					"--env", "CORGE", // OS exported
					"--env", "GRAULT=grault_key=grault_value", // value contains `=` char
					"--env", "GARPLY=", // OS exported
					"--env", "WALDO=", // not exported in OS
					"svc0",
					"env")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.All(
				expect.Contains(
					"\nFOO=foo1,foo2\n",
					"\nBAR=bar1 bar2\n",
					"\nBAZ=\n",
					"\nQUUX=quux2\n",
					"\nCORGE=corge-value-in-host\n",
					"\nGRAULT=grault_key=grault_value\n",
					"\nGARPLY=\n",
					"\nWALDO=\n"),
				expect.DoesNotContain("QUX"),
			)),
		},
	}

	userSubTest := &test.Case{
		Description: "with user",
		SubTests:    []*test.Case{},
	}

	userCasesMap := map[string]string{
		"":             "uid=0(root) gid=0(root)",
		"1000":         "uid=1000 gid=0(root)",
		"1000:users":   "uid=1000 gid=100(users)",
		"guest":        "uid=405(guest) gid=100(users)",
		"nobody":       "uid=65534(nobody) gid=65534(nobody)",
		"nobody:users": "uid=65534(nobody) gid=100(users)",
	}

	for k, v := range userCasesMap {
		userSubTest.SubTests = append(userSubTest.SubTests, &test.Case{
			Description: k + " " + v,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				args := []string{"compose", "-f", data.Labels().Get("YAMLPath"), "exec", "-i=false", "--no-TTY"}
				if k != "" {
					args = append(args, "--user", k)
				}
				args = append(args, "svc0", "id")
				return helpers.Command(args...)
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains(v)),
		})
	}

	testCase.SubTests = append(testCase.SubTests, userSubTest)

	testCase.Run(t)
}

func TestComposeExecTTY(t *testing.T) {
	const expectedOutput = "speed 38400 baud"
	dockerComposeYAML := fmt.Sprintf(`
version: '3.1'

services:
  svc0:
    image: %s
  svc1:
    image: %s
`, testutil.CommonImage, testutil.CommonImage)

	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		yamlPath := data.Temp().Save(dockerComposeYAML, "compose.yaml")
		data.Labels().Set("YAMLPath", yamlPath)
		helpers.Ensure(
			"compose",
			"-f",
			yamlPath,
			"run",
			"-d",
			"-i=false",
			"--name",
			data.Identifier(),
			"svc0",
			"sleep",
			"1h",
		)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		// FIXME?
		// similar, other test does *also* remove the container
		helpers.Anyhow("compose", "-f", data.Labels().Get("YAMLPath"), "down", "-v")
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "stty exec",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("compose", "-f", data.Labels().Get("YAMLPath"), "exec", "svc0", "stty")
				cmd.WithPseudoTTY()
				return cmd
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains(expectedOutput)),
		},
		{
			Description: "-i=false stty exec",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("compose", "-f", data.Labels().Get("YAMLPath"), "exec", "-i=false", "svc0", "stty")
				cmd.WithPseudoTTY()
				return cmd
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains(expectedOutput)),
		},
		{
			Description: "--no-TTY stty exec",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("compose", "-f", data.Labels().Get("YAMLPath"), "exec", "--no-TTY", "svc0", "stty")
				cmd.WithPseudoTTY()
				return cmd
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			Description: "-i=false --no-TTY stty exec",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command(
					"compose",
					"-f",
					data.Labels().Get("YAMLPath"),
					"exec",
					"-i=false",
					"--no-TTY",
					"svc0",
					"stty",
				)
				cmd.WithPseudoTTY()
				return cmd
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
	}

	testCase.Run(t)
}

func TestComposeExecWithIndex(t *testing.T) {
	dockerComposeYAML := fmt.Sprintf(`
version: '3.1'

services:
  svc0:
    image: %s
    command: "sleep infinity"
    deploy:
      replicas: 3
`, testutil.CommonImage)

	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		yamlPath := data.Temp().Save(dockerComposeYAML, "compose.yaml")
		data.Labels().Set("YAMLPath", yamlPath)
		data.Labels().Set("projectName", strings.ToLower(filepath.Base(data.Temp().Dir())))

		helpers.Ensure("compose", "-f", yamlPath, "up", "-d", "svc0")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down", "-v")
	}

	for _, index := range []string{"1", "2", "3"} {
		testCase.SubTests = append(testCase.SubTests, &test.Case{
			Description: index,
			Setup: func(data test.Data, helpers test.Helpers) {
				// try 5 times to ensure that results are stable
				for range 5 {
					cmds := []string{
						"compose",
						"-f",
						data.Labels().Get("YAMLPath"),
						"exec",
						"-i=false",
						"--no-TTY",
						"--index",
						index,
						"svc0",
					}

					hsts := helpers.Capture(append(cmds, "cat", "/etc/hosts")...)
					ips := helpers.Capture(append(cmds, "ip", "addr", "show", "dev", "eth0")...)

					var (
						expectIP string
						realIP   string
					)
					name := fmt.Sprintf("%s-svc0-%s", data.Labels().Get("projectName"), index)
					host := fmt.Sprintf("%s.%s_default", name, data.Labels().Get("projectName"))
					if nerdtest.IsDocker() {
						host = strings.TrimSpace(helpers.Capture("ps", "--filter", "name="+name, "--format", "{{.ID}}"))
					}

					lines := strings.Split(hsts, "\n")
					for _, line := range lines {
						if !strings.Contains(line, host) {
							continue
						}
						fields := strings.Fields(line)
						if len(fields) == 0 {
							continue
						}
						expectIP = fields[0]
					}

					var ip string
					lines = strings.Split(ips, "\n")
					for _, line := range lines {
						if !strings.Contains(line, "inet ") {
							continue
						}
						fields := strings.Fields(line)
						if len(fields) <= 1 {
							continue
						}
						ip = strings.Split(fields[1], "/")[0]
						break
					}

					pip := net.ParseIP(ip)

					assert.Assert(helpers.T(), pip != nil, "fail to get the real ip address")
					realIP = pip.String()

					assert.Equal(helpers.T(), realIP, expectIP)
				}
			},
		})
	}

	testCase.Run(t)
}
