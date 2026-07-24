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
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/portlock"
	"github.com/containerd/nerdctl/v2/pkg/testutil/testregistry"
)

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
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", "-v", data.Identifier())
				helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down", "-v")
			},
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
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", "-v", data.Identifier())
				helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down", "-v")
			},
		},
	}

	testCase.Run(t)
}

func TestComposeRunWithServicePorts(t *testing.T) {
	testCase := nerdtest.Setup()

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

		// specify the name of container in order to remove
		// TODO: when `compose rm` is implemented, replace it.
		cmd := helpers.Command("compose", "-f", composePath, "run", "--service-ports", "--name", data.Identifier(), "web")
		cmd.WithPseudoTTY()
		cmd.Background()
		nerdtest.EnsureContainerStarted(helpers, data.Identifier())
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", "-v", data.Identifier())
		if composeYAML := data.Labels().Get("composeYAML"); composeYAML != "" {
			helpers.Anyhow("compose", "-f", composeYAML, "down", "-v")
		}
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

		// specify the name of container in order to remove
		// TODO: when `compose rm` is implemented, replace it.
		cmd := helpers.Command("compose", "-f", composePath, "run", "--publish", fmt.Sprintf("%d:80", hostPort), "--name", data.Identifier(), "web")
		cmd.WithPseudoTTY()
		cmd.Background()
		nerdtest.EnsureContainerStarted(helpers, data.Identifier())
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", "-v", data.Identifier())
		if composeYAML := data.Labels().Get("composeYAML"); composeYAML != "" {
			helpers.Anyhow("compose", "-f", composeYAML, "down", "-v")
		}
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

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", "-v", data.Identifier())
		helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down", "-v")
	}

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

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", "-v", data.Identifier())
		helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down", "-v")
	}

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

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", "-v", data.Identifier())
		helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down", "-v")
	}

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

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", "-v", data.Identifier())
		helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down", "-v")
	}

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

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", "-v", data.Identifier())
		helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down", "-v")
	}

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

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", "-v", data.Identifier())
		helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down", "-v")
	}

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

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", "-v", data.Identifier())
		helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down", "-v")
	}

	testCase.Run(t)
}

func TestComposePushAndPullWithCosignVerify(t *testing.T) {
	testutil.RequireExecutable(t, "cosign")
	testutil.DockerIncompatible(t)
	testutil.RequiresBuild(t)
	testutil.RegisterBuildCacheCleanup(t)
	t.Parallel()

	base := testutil.NewBase(t)
	base.Env = append(base.Env, "COSIGN_PASSWORD=1")

	keyPair := helpers.NewCosignKeyPair(t, "cosign-key-pair", "1")
	reg := testregistry.NewWithNoAuth(base, 0, false)
	t.Cleanup(func() {
		keyPair.Cleanup()
		reg.Cleanup(nil)
	})

	tID := testutil.Identifier(t)
	testImageRefPrefix := fmt.Sprintf("127.0.0.1:%d/%s/", reg.Port, tID)

	var (
		imageSvc0 = testImageRefPrefix + "composebuild_svc0"
		imageSvc1 = testImageRefPrefix + "composebuild_svc1"
		imageSvc2 = testImageRefPrefix + "composebuild_svc2"
	)

	dockerComposeYAML := fmt.Sprintf(`
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
`, imageSvc0, keyPair.PublicKey, keyPair.PrivateKey,
		imageSvc1, keyPair.PrivateKey, imageSvc2)

	dockerfile := fmt.Sprintf(`FROM %s`, testutil.CommonImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	comp.WriteFile("Dockerfile", dockerfile)

	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	// 1. build both services/images
	base.ComposeCmd("-f", comp.YAMLFullPath(), "build").AssertOK()
	// 2. compose push with cosign for svc0/svc1, (and none for svc2)
	base.ComposeCmd("-f", comp.YAMLFullPath(), "push").AssertOK()
	// 3. compose pull with cosign
	base.ComposeCmd("-f", comp.YAMLFullPath(), "pull", "svc0").AssertOK()   // key match
	base.ComposeCmd("-f", comp.YAMLFullPath(), "pull", "svc1").AssertFail() // key mismatch
	base.ComposeCmd("-f", comp.YAMLFullPath(), "pull", "svc2").AssertOK()   // verify passed
	// 4. compose run
	const sttyPartialOutput = "speed 38400 baud"
	// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
	// unbuffer(1) can be installed with `apt-get install expect`.
	unbuffer := []string{"unbuffer"}
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(), "run", "svc0").AssertOutContains(sttyPartialOutput) // key match
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(), "run", "svc1").AssertFail()                         // key mismatch
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(), "run", "svc2").AssertOutContains(sttyPartialOutput) // verify passed
	// 5. compose up
	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "svc0").AssertOK()   // key match
	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "svc1").AssertFail() // key mismatch
	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "svc2").AssertOK()   // verify passed
}
