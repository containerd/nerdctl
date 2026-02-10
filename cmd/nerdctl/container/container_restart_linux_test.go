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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestRestart(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--name", testutil.Identifier(t), testutil.NginxAlpineImage, "sleep", nerdtest.Infinity)
		nerdtest.EnsureContainerStarted(helpers, testutil.Identifier(t))

		inspect := nerdtest.InspectContainer(helpers, testutil.Identifier(t))
		data.Labels().Set("pid", strconv.Itoa(inspect.State.Pid))

		helpers.Ensure("restart", testutil.Identifier(t))
		nerdtest.EnsureContainerStarted(helpers, testutil.Identifier(t))
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", testutil.Identifier(t))
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("inspect", testutil.Identifier(t))
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, t tig.T) {
				var dc []dockercompat.Container

				err := json.Unmarshal([]byte(stdout), &dc)
				assert.NilError(t, err)
				assert.Equal(t, 1, len(dc))

				assert.Assert(t, data.Labels().Get("pid") != strconv.Itoa(dc[0].State.Pid))
			},
		}
	}

	testCase.Run(t)
}

func TestRestartPIDContainer(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		baseContainerName := testutil.Identifier(t)
		helpers.Ensure("run", "-d", "--name", baseContainerName, testutil.AlpineImage, "sleep", nerdtest.Infinity)
		nerdtest.EnsureContainerStarted(helpers, baseContainerName)

		sharedContainerName := fmt.Sprintf("%s-shared", baseContainerName)
		helpers.Ensure("run", "-d", "--name", sharedContainerName, fmt.Sprintf("--pid=container:%s", baseContainerName), testutil.AlpineImage, "sleep", nerdtest.Infinity)
		nerdtest.EnsureContainerStarted(helpers, sharedContainerName)

		helpers.Ensure("restart", baseContainerName)
		nerdtest.EnsureContainerStarted(helpers, baseContainerName)
		helpers.Ensure("restart", sharedContainerName)
		nerdtest.EnsureContainerStarted(helpers, sharedContainerName)

		// output format : <inode number> /proc/1/ns/pid
		// example output: 4026532581 /proc/1/ns/pid
		basePSResult := helpers.Capture("exec", baseContainerName, "ls", "-Li", "/proc/1/ns/pid")
		baseOutput := strings.TrimSpace(basePSResult)

		data.Labels().Set("baseContainerName", baseContainerName)
		data.Labels().Set("sharedContainerName", sharedContainerName)
		data.Labels().Set("baseOutput", baseOutput)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Labels().Get("baseContainerName"))
		helpers.Anyhow("rm", "-f", data.Labels().Get("sharedContainerName"))
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("exec", data.Labels().Get("sharedContainerName"), "ls", "-Li", "/proc/1/ns/pid")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, t tig.T) {
				assert.Equal(t, strings.TrimSpace(stdout), data.Labels().Get("baseOutput"))
			},
		}
	}

	testCase.Run(t)
}

func TestRestartIPCContainer(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		const shmSize = "32m"
		baseContainerName := testutil.Identifier(t)
		helpers.Ensure("run", "-d", "--shm-size", shmSize, "--ipc", "shareable", "--name", baseContainerName, testutil.AlpineImage, "sleep", nerdtest.Infinity)
		nerdtest.EnsureContainerStarted(helpers, baseContainerName)

		sharedContainerName := fmt.Sprintf("%s-shared", baseContainerName)
		helpers.Ensure("run", "-d", "--name", sharedContainerName, fmt.Sprintf("--ipc=container:%s", baseContainerName), testutil.AlpineImage, "sleep", nerdtest.Infinity)
		nerdtest.EnsureContainerStarted(helpers, sharedContainerName)

		helpers.Ensure("stop", baseContainerName)
		helpers.Ensure("stop", sharedContainerName)

		helpers.Ensure("restart", baseContainerName)
		nerdtest.EnsureContainerStarted(helpers, baseContainerName)
		helpers.Ensure("restart", sharedContainerName)
		nerdtest.EnsureContainerStarted(helpers, sharedContainerName)

		baseShmSizeResult := helpers.Capture("exec", baseContainerName, "/bin/grep", "shm", "/proc/self/mounts")
		baseOutput := strings.TrimSpace(baseShmSizeResult)

		data.Labels().Set("baseContainerName", baseContainerName)
		data.Labels().Set("sharedContainerName", sharedContainerName)
		data.Labels().Set("baseOutput", baseOutput)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Labels().Get("baseContainerName"))
		helpers.Anyhow("rm", "-f", data.Labels().Get("sharedContainerName"))
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("exec", data.Labels().Get("sharedContainerName"), "/bin/grep", "shm", "/proc/self/mounts")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, t tig.T) {
				assert.Equal(t, strings.TrimSpace(stdout), data.Labels().Get("baseOutput"))
			},
		}
	}

	testCase.Run(t)
}

func TestRestartWithTime(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		containerName := testutil.Identifier(t)
		helpers.Ensure("run", "-d", "--name", containerName, testutil.AlpineImage, "sleep", nerdtest.Infinity)
		nerdtest.EnsureContainerStarted(helpers, containerName)

		inspect := nerdtest.InspectContainer(helpers, containerName)
		pid := inspect.State.Pid

		data.Labels().Set("containerName", containerName)
		data.Labels().Set("pid", strconv.Itoa(pid))
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Labels().Get("containerName"))
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		data.Labels().Set("timePreRestart", time.Now().Format(time.RFC3339))
		return helpers.Command("restart", "-t", "5", data.Labels().Get("containerName"))
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, t tig.T) {
				timePostRestart := time.Now()
				timePreRestart, err := time.Parse(time.RFC3339, data.Labels().Get("timePreRestart"))
				assert.NilError(t, err)
				// ensure that stop took at least 5 seconds
				assert.Assert(t, timePostRestart.Sub(timePreRestart) >= time.Second*5)

				inspect := nerdtest.InspectContainer(helpers, data.Labels().Get("containerName"))
				assert.Assert(t, strconv.Itoa(inspect.State.Pid) != data.Labels().Get("pid"))
			},
		}
	}

	testCase.Run(t)
}

func TestRestartWithSignal(t *testing.T) {
	testCase := nerdtest.Setup()

	// FIXME: gomodjail signal handling is not working yet: https://github.com/AkihiroSuda/gomodjail/issues/51
	testCase.Require = require.Not(nerdtest.Gomodjail)

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		cmd := nerdtest.RunSigProxyContainer(nerdtest.SigUsr1, false, nil, data, helpers)
		// Capture the current pid
		data.Labels().Set("oldpid", strconv.Itoa(nerdtest.InspectContainer(helpers, data.Identifier()).State.Pid))
		// Send the signal
		helpers.Ensure("restart", "--signal", "SIGUSR1", data.Identifier())
		return cmd
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			// Check the container did indeed exit
			ExitCode: 137,
			Output: expect.All(
				// Check that we saw SIGUSR1 inside the container
				expect.Contains(nerdtest.SignalCaught),
				func(stdout string, t tig.T) {
					// Ensure the container was restarted
					nerdtest.EnsureContainerStarted(helpers, data.Identifier())
					// Check the new pid is different
					newpid := strconv.Itoa(nerdtest.InspectContainer(helpers, data.Identifier()).State.Pid)
					assert.Assert(helpers.T(), newpid != data.Labels().Get("oldpid"))
				},
			),
		}
	}

	testCase.Run(t)
}
