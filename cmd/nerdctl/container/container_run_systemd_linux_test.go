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

package container

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestRunWithSystemdAlways(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Docker)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Labels().Set("containerName", testutil.Identifier(t))
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		containerName := data.Labels().Get("containerName")
		helpers.Anyhow("container", "rm", "-f", containerName)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "should mount cgroup filesystem as rw",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				containerName := data.Labels().Get("containerName")
				return helpers.Command("run", "--name", containerName, "--systemd=always", "--entrypoint=/bin/bash", testutil.UbuntuImage, "-c", "mount | grep cgroup")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("(rw,")),
		},
		{
			Description: "should expose SIGTERM+3 stop signal label",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				containerName := data.Labels().Get("containerName")
				return helpers.Command("inspect", "--format", "{{json .Config.Labels}}", containerName)
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("SIGRTMIN+3")),
		},
	}

	testCase.Run(t)
}

func TestRunWithSystemdTrueEnabled(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.All(
		require.Amd64,
		require.Not(nerdtest.Docker),
	)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--name", data.Identifier(), "--systemd=true", "--entrypoint=/sbin/init", testutil.SystemdImage)
		nerdtest.EnsureContainerStarted(helpers, data.Identifier())
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("container", "rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		// should expose SIGTERM+3 stop signal labels
		helpers.Command("inspect", "--format", "{{json .Config.Labels}}", data.Identifier()).
			Run(&test.Expected{
				ExitCode: expect.ExitCodeSuccess,
				Output:   expect.Contains("SIGRTMIN+3"),
			})

		// Poll for systemd to become ready using the same pattern as EnsureContainerStarted
		const maxRetry = 30
		const sleep = time.Second
		systemdReady := false

		for i := 0; i < maxRetry && !systemdReady; i++ {
			helpers.Command("exec", data.Identifier(), "sh", "-c", "--", "systemctl is-system-running").
				Run(&test.Expected{
					ExitCode: expect.ExitCodeNoCheck,
					Output: func(stdout string, t tig.T) {
						// Silent return on error or empty output
						if stdout == "" {
							return
						}
						// Check for systemd ready states
						if strings.Contains(stdout, "running") || strings.Contains(stdout, "degraded") {
							systemdReady = true
						}
					},
				})
			time.Sleep(sleep)
		}

		if !systemdReady {
			helpers.T().Log("systemd did not become ready after 30 seconds")
			helpers.T().FailNow()
		}

		return helpers.Command("exec", data.Identifier(), "sh", "-c", "--", "systemctl list-jobs")
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("jobs"))

	testCase.Run(t)
}

func TestRunWithSystemdTrueDisabled(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.All(
		require.Amd64,
		require.Not(nerdtest.Docker),
	)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Labels().Set("containerName", testutil.Identifier(t))
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		containerName := data.Labels().Get("containerName")
		helpers.Anyhow("container", "rm", "-f", containerName)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		containerName := data.Labels().Get("containerName")
		return helpers.Command("run", "--name", containerName, "--systemd=true", "--entrypoint=/bin/bash", testutil.SystemdImage, "-c", "systemctl list-jobs")
	}
	testCase.Expected = test.Expects(1, []error{errors.New("System has not been booted with systemd as init system")}, nil)

	testCase.Run(t)
}

func TestRunWithSystemdFalse(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Docker)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Labels().Set("containerName", testutil.Identifier(t))
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		containerName := data.Labels().Get("containerName")
		helpers.Anyhow("container", "rm", "-f", containerName)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "should mount cgroup filesystem as ro",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				containerName := data.Labels().Get("containerName")
				return helpers.Command("run", "--name", containerName, "--systemd=false", "--entrypoint=/bin/bash", testutil.UbuntuImage, "-c", "mount | grep cgroup")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("(ro,")),
		},
		{
			Description: "should expose SIGTERM stop signal label",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				containerName := data.Labels().Get("containerName")
				return helpers.Command("inspect", "--format", "{{json .Config.Labels}}", containerName)
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("SIGTERM")),
		},
	}

	testCase.Run(t)
}

func TestRunWithNoSystemd(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Docker)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Labels().Set("containerName", testutil.Identifier(t))
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		containerName := data.Labels().Get("containerName")
		helpers.Anyhow("container", "rm", "-f", containerName)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "should mount cgroup filesystem as ro",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				containerName := data.Labels().Get("containerName")
				return helpers.Command("run", "--name", containerName, "--entrypoint=/bin/bash", testutil.UbuntuImage, "-c", "mount | grep cgroup")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("(ro,")),
		},
		{
			Description: "should expose SIGTERM stop signal label",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				containerName := data.Labels().Get("containerName")
				return helpers.Command("inspect", "--format", "{{json .Config.Labels}}", containerName)
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("SIGTERM")),
		},
	}

	testCase.Run(t)
}

func TestRunWithSystemdPrivilegedError(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.All(
		require.Amd64,
		require.Not(nerdtest.Docker),
	)

	testCase.Command = test.Command("run", "--privileged", "--rm", "--systemd=always", "--entrypoint=/sbin/init", testutil.SystemdImage)
	testCase.Expected = test.Expects(1, []error{errors.New("if --privileged is used with systemd `--security-opt privileged-without-host-devices` must also be used")}, nil)

	testCase.Run(t)
}

func TestRunWithSystemdPrivilegedSuccess(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.All(
		require.Amd64,
		require.Not(nerdtest.Docker),
	)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		containerName := testutil.Identifier(t)
		data.Labels().Set("containerName", containerName)
		helpers.Ensure("run", "-d", "--name", containerName, "--privileged", "--security-opt", "privileged-without-host-devices", "--systemd=true", "--entrypoint=/sbin/init", testutil.SystemdImage)
		nerdtest.EnsureContainerStarted(helpers, containerName)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		containerName := data.Labels().Get("containerName")
		helpers.Anyhow("container", "rm", "-f", containerName)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		containerName := data.Labels().Get("containerName")
		return helpers.Command("inspect", "--format", "{{json .Config.Labels}}", containerName)
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("SIGRTMIN+3"))

	testCase.Run(t)
}
