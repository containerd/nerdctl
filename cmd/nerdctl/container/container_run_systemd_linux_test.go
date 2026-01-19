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
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

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
		containerName := testutil.Identifier(t)
		data.Labels().Set("containerName", containerName)
		helpers.Ensure("run", "-d", "--name", containerName, "--systemd=true", "--entrypoint=/sbin/init", testutil.SystemdImage)
		nerdtest.EnsureContainerStarted(helpers, containerName)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		containerName := data.Labels().Get("containerName")
		helpers.Anyhow("container", "rm", "-f", containerName)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "should expose SIGTERM+3 stop signal labels",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				containerName := data.Labels().Get("containerName")
				return helpers.Command("inspect", "--format", "{{json .Config.Labels}}", containerName)
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("SIGRTMIN+3")),
		},
		{
			Description: "waits for systemd to become ready and lists systemd jobs",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				containerName := data.Labels().Get("containerName")
				return helpers.Command("exec", containerName, "sh", "-c", "--", `tries=0

			until systemctl is-system-running >/dev/null 2>&1; do

				>&2 printf "Waiting for systemd to come up...\n"
				sleep 1s
				tries=$(( tries + 1))
				[ $tries -lt 10 ] || {
					>&2 printf "systemd failed to come up in a reasonable amount of time\n"
					exit 1
				}
			done
			systemctl list-jobs`)
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("jobs")),
		},
	}

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
