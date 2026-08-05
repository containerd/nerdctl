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
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/registry"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/portlock"
)

func composeRunCleanup() test.Butler {
	return func(data test.Data, helpers test.Helpers) {
		composePath := data.Temp().Path("compose.yaml")
		// Tigron runs cleanup before setup too. A fresh temp project has no
		// manifest or resources yet, so avoid waiting for the global compose lock.
		if _, err := os.Stat(composePath); os.IsNotExist(err) {
			return
		}
		// A background compose run holds the global compose lock. Stop its exact
		// test container first so the process exits before compose rm acquires it.
		helpers.Anyhow("stop", data.Identifier())
		helpers.Anyhow("compose", "-f", composePath, "rm", "-f", "-s", "-v")
		// Docker Compose excludes one-off containers from `compose rm`, while
		// nerdctl Compose selects every container with the project and service labels.
		// Remove the explicit `compose run --name` container in compatibility runs.
		if nerdtest.IsDocker() {
			helpers.Anyhow("rm", "-f", "-v", data.Identifier())
		}
		helpers.Anyhow("compose", "-f", composePath, "down", "-v")
	}
}

func TestComposeRun(t *testing.T) {
	const expectedOutput = "speed 38400 baud"

	dockerComposeYAML := fmt.Sprintf(`
services:
  alpine:
    image: %s
    entrypoint:
      - stty
`, testutil.CommonImage)

	testCase := nerdtest.Setup()

	testCase.SubTests = []*test.Case{
		{
			Description: "pty run",
			Setup: func(data test.Data, helpers test.Helpers) {
				data.Temp().Save(dockerComposeYAML, "compose.yaml")
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command(
					"compose",
					"-f",
					data.Temp().Path("compose.yaml"),
					"run",
					"--name",
					data.Identifier(),
					"alpine",
				)
				cmd.WithPseudoTTY()
				return cmd
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains(expectedOutput)),
			Cleanup:  composeRunCleanup(),
		},
		{
			Description: "pty run with --rm",
			Setup: func(data test.Data, helpers test.Helpers) {
				data.Temp().Save(dockerComposeYAML, "compose.yaml")
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command(
					"compose",
					"-f",
					data.Temp().Path("compose.yaml"),
					"run",
					"--name",
					data.Identifier(),
					"--rm",
					"alpine",
				)
				cmd.WithPseudoTTY()
				return cmd
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				// Ensure the container has been removed
				capt := helpers.Capture("ps", "-a", "--format=\"{{.Names}}\"")
				assert.Assert(t, !strings.Contains(capt, data.Identifier()), capt)

				return &test.Expected{
					Output: expect.Contains(expectedOutput),
				}
			},
			Cleanup: composeRunCleanup(),
		},
	}

	testCase.Run(t)
}

func TestComposeRunWithServicePorts(t *testing.T) {
	testCase := nerdtest.Setup()
	// A background compose run holds the global compose lock until cleanup.
	testCase.NoParallel = true
	cleanup := composeRunCleanup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		hostPort, err := portlock.Acquire(0)
		if err != nil {
			helpers.T().Log(fmt.Sprintf("Failed to acquire port: %v", err))
			helpers.T().FailNow()
		}

		dockerComposeYAML := fmt.Sprintf(`
services:
  web:
    image: %s
    ports:
      - %d:80
`, testutil.NginxAlpineImage, hostPort)

		composePath := data.Temp().Save(dockerComposeYAML, "compose.yaml")
		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)

		data.Labels().Set("composeYAML", composePath)
		data.Labels().Set("hostPort", strconv.Itoa(hostPort))

		cmd := helpers.Command("compose", "-f", composePath, "run", "--service-ports", "--name", data.Identifier(), "web")
		cmd.WithPseudoTTY()
		cmd.Background()
		nerdtest.EnsureContainerStarted(helpers, data.Identifier())
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		cleanup(data, helpers)
		if portStr := data.Labels().Get("hostPort"); portStr != "" {
			if port, err := strconv.Atoi(portStr); err == nil {
				_ = portlock.Release(port)
			}
		}
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("inspect", data.Identifier())
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, tt tig.T) {
				resp, err := nettestutil.HTTPGet(fmt.Sprintf("http://127.0.0.1:%s", data.Labels().Get("hostPort")), 5, false)
				assert.NilError(tt, err)
				defer resp.Body.Close()
				respBody, err := io.ReadAll(resp.Body)
				assert.NilError(tt, err)
				tt.Log(fmt.Sprintf("respBody=%q", respBody))
				assert.Assert(tt, strings.Contains(string(respBody), testutil.NginxAlpineIndexHTMLSnippet), fmt.Sprintf("respBody does not contain %q", testutil.NginxAlpineIndexHTMLSnippet))
			},
		}
	}

	testCase.Run(t)
}

func TestComposeRunWithPublish(t *testing.T) {
	testCase := nerdtest.Setup()
	// A background compose run holds the global compose lock until cleanup.
	testCase.NoParallel = true
	cleanup := composeRunCleanup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		hostPort, err := portlock.Acquire(0)
		if err != nil {
			helpers.T().Log(fmt.Sprintf("Failed to acquire port: %v", err))
			helpers.T().FailNow()
		}

		dockerComposeYAML := fmt.Sprintf(`
services:
  web:
    image: %s
`, testutil.NginxAlpineImage)

		composePath := data.Temp().Save(dockerComposeYAML, "compose.yaml")
		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)

		data.Labels().Set("composeYAML", composePath)
		data.Labels().Set("hostPort", strconv.Itoa(hostPort))

		cmd := helpers.Command("compose", "-f", composePath, "run", "--publish", fmt.Sprintf("%d:80", hostPort), "--name", data.Identifier(), "web")
		cmd.WithPseudoTTY()
		cmd.Background()
		nerdtest.EnsureContainerStarted(helpers, data.Identifier())
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		cleanup(data, helpers)
		if portStr := data.Labels().Get("hostPort"); portStr != "" {
			if port, err := strconv.Atoi(portStr); err == nil {
				_ = portlock.Release(port)
			}
		}
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("inspect", data.Identifier())
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, tt tig.T) {
				resp, err := nettestutil.HTTPGet(fmt.Sprintf("http://127.0.0.1:%s", data.Labels().Get("hostPort")), 5, false)
				assert.NilError(tt, err)
				defer resp.Body.Close()
				respBody, err := io.ReadAll(resp.Body)
				assert.NilError(tt, err)
				tt.Log(fmt.Sprintf("respBody=%q", respBody))
				assert.Assert(tt, strings.Contains(string(respBody), testutil.NginxAlpineIndexHTMLSnippet), fmt.Sprintf("respBody does not contain %q", testutil.NginxAlpineIndexHTMLSnippet))
			},
		}
	}

	testCase.Run(t)
}

func TestComposeRunWithEnv(t *testing.T) {
	const partialOutput = "bar"

	dockerComposeYAML := fmt.Sprintf(`
services:
  alpine:
    image: %s
    entrypoint:
      - sh
      - -c
      - "echo $$FOO"
`, testutil.CommonImage)

	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		composePath := data.Temp().Save(dockerComposeYAML, "compose.yaml")
		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		cmd := helpers.Command(
			"compose",
			"-f",
			data.Temp().Path("compose.yaml"),
			"run",
			"-e",
			"FOO=bar",
			"--name",
			data.Identifier(),
			"alpine",
		)
		cmd.WithPseudoTTY()
		return cmd
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains(partialOutput))

	testCase.Cleanup = composeRunCleanup()

	testCase.Run(t)
}

func TestComposeRunWithUser(t *testing.T) {
	const partialOutput = "5000"

	dockerComposeYAML := fmt.Sprintf(`
services:
  alpine:
    image: %s
    entrypoint:
      - id
      - -u
`, testutil.CommonImage)

	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		composePath := data.Temp().Save(dockerComposeYAML, "compose.yaml")
		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		cmd := helpers.Command(
			"compose",
			"-f",
			data.Temp().Path("compose.yaml"),
			"run",
			"--user",
			"5000",
			"--name",
			data.Identifier(),
			"alpine",
		)
		cmd.WithPseudoTTY()
		return cmd
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains(partialOutput))

	testCase.Cleanup = composeRunCleanup()

	testCase.Run(t)
}

func TestComposeRunWithWorkdir(t *testing.T) {
	const expectedOutput = "/tmp"

	dockerComposeYAML := fmt.Sprintf(`
services:
  alpine:
    image: %s
    entrypoint:
      - pwd
`, testutil.CommonImage)

	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		composePath := data.Temp().Save(dockerComposeYAML, "compose.yaml")
		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		cmd := helpers.Command(
			"compose",
			"-f",
			data.Temp().Path("compose.yaml"),
			"run",
			"--workdir",
			"/tmp",
			"--name",
			data.Identifier(),
			"alpine",
		)
		cmd.WithPseudoTTY()
		return cmd
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains(expectedOutput))

	testCase.Cleanup = composeRunCleanup()

	testCase.Run(t)
}

func TestComposeRunWithLabel(t *testing.T) {
	dockerComposeYAML := fmt.Sprintf(`
services:
  alpine:
    image: %s
    entrypoint:
      - echo
      - "dummy log"
    labels:
      - "foo=bar"
`, testutil.CommonImage)

	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		composePath := data.Temp().Save(dockerComposeYAML, "compose.yaml")
		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		cmd := helpers.Command(
			"compose",
			"-f",
			data.Temp().Path("compose.yaml"),
			"run",
			"--label",
			"foo=rab",
			"--label",
			"x=y",
			"--name",
			data.Identifier(),
			"alpine",
		)
		cmd.WithPseudoTTY()
		return cmd
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, tt tig.T) {
				container := nerdtest.InspectContainer(helpers, data.Identifier())
				assert.Assert(tt, container.Config != nil, "cannot fetch container config")
				assert.Equal(tt, container.Config.Labels["foo"], "rab")
				assert.Equal(tt, container.Config.Labels["x"], "y")
			},
		}
	}

	testCase.Cleanup = composeRunCleanup()

	testCase.Run(t)
}

func TestComposeRunWithArgs(t *testing.T) {
	const partialOutput = "hello world"

	dockerComposeYAML := fmt.Sprintf(`
services:
  alpine:
    image: %s
    entrypoint:
      - echo
`, testutil.CommonImage)

	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		composePath := data.Temp().Save(dockerComposeYAML, "compose.yaml")
		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		cmd := helpers.Command(
			"compose",
			"-f",
			data.Temp().Path("compose.yaml"),
			"run",
			"--name",
			data.Identifier(),
			"alpine",
			partialOutput,
		)
		cmd.WithPseudoTTY()
		return cmd
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains(partialOutput))

	testCase.Cleanup = composeRunCleanup()

	testCase.Run(t)
}

func TestComposeRunWithEntrypoint(t *testing.T) {
	const partialOutput = "hello world"

	dockerComposeYAML := fmt.Sprintf(`
services:
  alpine:
    image: %s
    entrypoint:
      - stty # should be changed
`, testutil.CommonImage)

	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		composePath := data.Temp().Save(dockerComposeYAML, "compose.yaml")
		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		cmd := helpers.Command(
			"compose",
			"-f",
			data.Temp().Path("compose.yaml"),
			"run",
			"--entrypoint",
			"echo",
			"--name",
			data.Identifier(),
			"alpine",
			partialOutput,
		)
		cmd.WithPseudoTTY()
		return cmd
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains(partialOutput))

	testCase.Cleanup = composeRunCleanup()

	testCase.Run(t)
}

func TestComposeRunWithVolume(t *testing.T) {
	dockerComposeYAML := fmt.Sprintf(`
services:
  alpine:
    image: %s
    entrypoint:
    - stty # no meaning, just put any command
`, testutil.CommonImage)

	const destinationDir = "/data"

	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		composePath := data.Temp().Save(dockerComposeYAML, "compose.yaml")
		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		volumeFlagStr := fmt.Sprintf("%s:%s", data.Temp().Path(), destinationDir)
		cmd := helpers.Command(
			"compose",
			"-f",
			data.Temp().Path("compose.yaml"),
			"run",
			"--volume",
			volumeFlagStr,
			"--name",
			data.Identifier(),
			"alpine",
		)
		cmd.WithPseudoTTY()
		return cmd
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, tt tig.T) {
				container := nerdtest.InspectContainer(helpers, data.Identifier())
				errMsg := fmt.Sprintf("test failed, cannot find volume: %v", container.Mounts)
				assert.Assert(tt, container.Mounts != nil, errMsg)
				assert.Assert(tt, len(container.Mounts) == 1, errMsg)
				assert.Assert(tt, container.Mounts[0].Source == data.Temp().Path(), errMsg)
				assert.Assert(tt, container.Mounts[0].Destination == destinationDir, errMsg)
			},
		}
	}

	testCase.Cleanup = composeRunCleanup()

	testCase.Run(t)
}

func TestComposePushAndPullWithCosignVerify(t *testing.T) {
	const sttyPartialOutput = "speed 38400 baud"

	testCase := nerdtest.Setup()

	testCase.Require = require.All(
		require.Binary("cosign"),
		require.Not(nerdtest.Docker),
		nerdtest.Build,
		nerdtest.Registry,
	)

	testCase.Env["COSIGN_PASSWORD"] = "1"

	dockerfile := fmt.Sprintf("FROM %s", testutil.CommonImage)

	var reg *registry.Server
	var composeYAML string

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		pri, pub := nerdtest.GenerateCosignKeyPair(data, helpers, "1")
		reg = nerdtest.RegistryWithNoAuth(data, helpers, 0, false)
		reg.Setup(data, helpers)

		prefix := fmt.Sprintf("127.0.0.1:%d/%s/", reg.Port, data.Identifier())
		composeYAML = fmt.Sprintf(`
services:
  svc0:
    build: .
    image: %s
    x-nerdctl-verify: cosign
    x-nerdctl-cosign-public-key: %s
    x-nerdctl-sign: cosign
    x-nerdctl-cosign-private-key: %s
    entrypoint:
      - stty
  svc1:
    build: .
    image: %s
    x-nerdctl-verify: cosign
    x-nerdctl-cosign-public-key: dummy_pub_key
    x-nerdctl-sign: cosign
    x-nerdctl-cosign-private-key: %s
    entrypoint:
      - stty
  svc2:
    build: .
    image: %s
    x-nerdctl-verify: none
    x-nerdctl-sign: none
    entrypoint:
      - stty
`, prefix+"composebuild_svc0", pub, pri, prefix+"composebuild_svc1", pri, prefix+"composebuild_svc2")

		data.Temp().Save(composeYAML, "compose.yaml")
		data.Temp().Save(dockerfile, "Dockerfile")

		composePath := data.Temp().Path("compose.yaml")
		// Build both services/images and push, signing svc0/svc1 with cosign (svc2 unsigned).
		helpers.Ensure("compose", "-f", composePath, "build")
		helpers.Ensure("compose", "-f", composePath, "push")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		composeRunCleanup()(data, helpers)
		if reg != nil {
			reg.Cleanup(data, helpers)
		}
	}

	// Each subtest re-materializes the compose project (the signed images live in the
	// shared registry set up above) and exercises one verify scenario:
	// svc0 verifies against the matching key, svc1 against a mismatching key (must fail),
	// svc2 is not verified.
	subTest := func(description, op, svc string, tty bool, expected test.Manager) *test.Case {
		return &test.Case{
			Description: description,
			Setup: func(data test.Data, helpers test.Helpers) {
				data.Temp().Save(composeYAML, "compose.yaml")
				data.Temp().Save(dockerfile, "Dockerfile")
			},
			Cleanup: composeRunCleanup(),
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("compose", "-f", data.Temp().Path("compose.yaml"), op, svc)
				if tty {
					// stty (the entrypoint) requires a tty, which `run -t` provides.
					cmd.WithPseudoTTY()
				}
				return cmd
			},
			Expected: expected,
		}
	}

	success := test.Expects(expect.ExitCodeSuccess, nil, nil)
	fail := test.Expects(expect.ExitCodeGenericFail, nil, nil)
	successWithOutput := test.Expects(expect.ExitCodeSuccess, nil, expect.Contains(sttyPartialOutput))

	testCase.SubTests = []*test.Case{
		subTest("compose pull svc0 (key match)", "pull", "svc0", false, success),
		subTest("compose pull svc1 (key mismatch)", "pull", "svc1", false, fail),
		subTest("compose pull svc2 (verify none)", "pull", "svc2", false, success),
		subTest("compose run svc0 (key match)", "run", "svc0", true, successWithOutput),
		subTest("compose run svc1 (key mismatch)", "run", "svc1", true, fail),
		subTest("compose run svc2 (verify none)", "run", "svc2", true, successWithOutput),
		subTest("compose up svc0 (key match)", "up", "svc0", false, success),
		subTest("compose up svc1 (key mismatch)", "up", "svc1", false, fail),
		subTest("compose up svc2 (verify none)", "up", "svc2", false, success),
	}

	testCase.Run(t)
}
