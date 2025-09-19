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
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/healthcheck"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestContainerHealthCheckBasic(t *testing.T) {
	testCase := nerdtest.Setup()

	// Docker CLI does not provide a standalone healthcheck command.
	testCase.Require = require.Not(nerdtest.Docker)

	// Skip systemd tests in rootless environment to bypass dbus permission issues
	if rootlessutil.IsRootless() {
		t.Skip("systemd healthcheck tests are skipped in rootless environment")
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "Container does not exist",
			Command:     test.Command("container", "healthcheck", "non-existent"),
			Expected:    test.Expects(1, []error{errors.New("no such container non-existent")}, nil),
		},
		{
			Description: "Missing health check config",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					testutil.CommonImage, "sleep", nerdtest.Infinity)
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("container", "healthcheck", data.Identifier())
			},
			Expected: test.Expects(1, []error{errors.New("container has no health check configured")}, nil),
		},
		{
			Description: "Basic health check success",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "echo healthy",
					"--health-interval", "45s",
					"--health-timeout", "30s",
					testutil.CommonImage, "sleep", nerdtest.Infinity)
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("container", "healthcheck", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(func(stdout string, t tig.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						h := inspect.State.Health
						debug, _ := json.MarshalIndent(h, "", "  ")
						t.Log(string(debug))
						assert.Assert(t, h != nil, "expected health state to be present")
						assert.Equal(t, healthcheck.Healthy, h.Status)
						assert.Equal(t, 0, h.FailingStreak)
						assert.Assert(t, len(h.Log) > 0, "expected at least one health check log entry")
					}),
				}
			},
		},
		{
			Description: "Health check on stopped container",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "echo healthy",
					"--health-interval", "3s",
					testutil.CommonImage, "sleep", "2")
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
				helpers.Ensure("stop", data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("container", "healthcheck", data.Identifier())
			},
			Expected: test.Expects(1, []error{errors.New("container is not running (status: stopped)")}, nil),
		},
		{
			Description: "Health check without task",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("create", "--name", data.Identifier(),
					"--health-cmd", "echo healthy",
					testutil.CommonImage, "sleep", nerdtest.Infinity)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("container", "healthcheck", data.Identifier())
			},
			Expected: test.Expects(1, []error{errors.New("failed to get container task: no running task found")}, nil),
		},
	}

	testCase.Run(t)
}

func TestContainerHealthCheckAdvance(t *testing.T) {
	testCase := nerdtest.Setup()

	// Docker CLI does not provide a standalone healthcheck command.
	testCase.Require = require.Not(nerdtest.Docker)

	// Skip systemd tests in rootless environment to bypass dbus permission issues
	if rootlessutil.IsRootless() {
		t.Skip("systemd healthcheck tests are skipped in rootless environment")
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "Health check timeout scenario",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "sleep 10",
					"--health-timeout", "2s",
					"--health-interval", "1s",
					testutil.CommonImage, "sleep", nerdtest.Infinity)
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("container", "healthcheck", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(func(stdout string, t tig.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						h := inspect.State.Health
						debug, _ := json.MarshalIndent(h, "", "  ")
						t.Log(string(debug))
						assert.Assert(t, h != nil, "expected health state")
						assert.Equal(t, h.FailingStreak, 1)
						assert.Assert(t, len(inspect.State.Health.Log) > 0, "expected health log to have entries")
						last := inspect.State.Health.Log[0]
						assert.Equal(t, -1, last.ExitCode)
					}),
				}
			},
		},
		{
			Description: "Health check failing streak behavior",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "exit 1",
					"--health-interval", "1s",
					"--health-retries", "2",
					testutil.CommonImage, "sleep", nerdtest.Infinity)
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				// Run healthcheck twice to ensure failing streak
				for i := 0; i < 2; i++ {
					helpers.Ensure("container", "healthcheck", data.Identifier())
					time.Sleep(2 * time.Second)
				}
				return helpers.Command("inspect", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(func(stdout string, t tig.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						h := inspect.State.Health
						debug, _ := json.MarshalIndent(h, "", "  ")
						t.Log(string(debug))
						assert.Assert(t, h != nil, "expected health state")
						assert.Equal(t, h.Status, healthcheck.Unhealthy)
						assert.Equal(t, h.FailingStreak, 2)
					}),
				}
			},
		},
		{
			Description: "Health check with start period",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "exit 1",
					"--health-interval", "1s",
					"--health-start-period", "60s",
					"--health-retries", "2",
					testutil.CommonImage, "sleep", nerdtest.Infinity)
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("container", "healthcheck", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(func(stdout string, t tig.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						h := inspect.State.Health
						debug, _ := json.MarshalIndent(h, "", "  ")
						t.Log(string(debug))
						assert.Assert(t, h != nil, "expected health state")
						assert.Equal(t, h.Status, healthcheck.Starting)
						assert.Equal(t, h.FailingStreak, 0)
					}),
				}
			},
		},
		{
			Description: "Health check with invalid command",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "not-a-real-cmd",
					"--health-interval", "1s",
					"--health-retries", "1",
					testutil.CommonImage, "sleep", nerdtest.Infinity)
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("container", "healthcheck", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(func(stdout string, t tig.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						h := inspect.State.Health
						debug, _ := json.MarshalIndent(h, "", "  ")
						t.Log(string(debug))
						assert.Assert(t, h != nil, "expected health state")
						assert.Equal(t, h.Status, healthcheck.Unhealthy)
						assert.Equal(t, h.FailingStreak, 1)
					}),
				}
			},
		},
		{
			Description: "No healthcheck flag disables health status",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--no-healthcheck", testutil.CommonImage, "sleep", nerdtest.Infinity)
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("inspect", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(func(stdout string, t tig.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						assert.Assert(t, inspect.State.Health == nil, "expected health to be nil with --no-healthcheck")
					}),
				}
			},
		},
		{
			Description: "Healthcheck using CMD-SHELL format",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "echo shell-format", "--health-interval", "1s",
					testutil.CommonImage, "sleep", nerdtest.Infinity)
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("container", "healthcheck", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(func(_ string, t tig.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						h := inspect.State.Health
						debug, _ := json.MarshalIndent(h, "", "  ")
						t.Log(string(debug))
						assert.Assert(t, h != nil, "expected health state")
						assert.Equal(t, h.Status, healthcheck.Healthy)
						assert.Assert(t, len(h.Log) > 0)
						assert.Assert(t, strings.Contains(h.Log[0].Output, "shell-format"))
					}),
				}
			},
		},
		{
			Description: "Health check uses container environment variables",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--env", "MYVAR=test-value",
					"--health-cmd", "echo $MYVAR",
					"--health-interval", "1s",
					"--health-timeout", "1s",
					testutil.CommonImage, "sleep", nerdtest.Infinity)
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("container", "healthcheck", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(func(stdout string, t tig.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						h := inspect.State.Health
						debug, _ := json.MarshalIndent(h, "", "  ")
						t.Log(string(debug))
						assert.Assert(t, h != nil, "expected health state")
						assert.Equal(t, h.Status, healthcheck.Healthy)
						assert.Assert(t, h.FailingStreak == 0)
						assert.Assert(t, strings.Contains(h.Log[0].Output, "test"), "expected health log output to contain 'test'")
					}),
				}
			},
		},
		{
			Description: "Health check respects container WorkingDir",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--workdir", "/tmp",
					"--health-cmd", "pwd",
					"--health-interval", "1s",
					"--health-timeout", "1s",
					testutil.CommonImage, "sleep", nerdtest.Infinity)
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("container", "healthcheck", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(func(stdout string, t tig.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						h := inspect.State.Health
						debug, _ := json.MarshalIndent(h, "", "  ")
						t.Log(string(debug))
						assert.Assert(t, h != nil, "expected health state")
						assert.Equal(t, h.Status, healthcheck.Healthy)
						assert.Equal(t, h.FailingStreak, 0)
						assert.Assert(t, strings.Contains(h.Log[0].Output, "/tmp"), "expected health log output to contain '/tmp'")
					}),
				}
			},
		},
		{
			Description: "Healthcheck emits large output repeatedly",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "yes X | head -c 60000",
					"--health-interval", "1s", "--health-timeout", "2s",
					testutil.CommonImage, "sleep", nerdtest.Infinity)
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				for i := 0; i < 3; i++ {
					helpers.Ensure("container", "healthcheck", data.Identifier())
					time.Sleep(2 * time.Second)
				}
				return helpers.Command("inspect", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(func(_ string, t tig.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						h := inspect.State.Health
						debug, _ := json.MarshalIndent(h, "", "  ")
						t.Log(string(debug))
						assert.Assert(t, h != nil, "expected health state")
						assert.Equal(t, h.Status, healthcheck.Healthy)
						assert.Assert(t, len(h.Log) >= 3, "expected at least 3 health log entries")
						for _, log := range h.Log {
							assert.Assert(t, len(log.Output) >= 1024, fmt.Sprintf("each output should be >= 1024 bytes, was: %s", log.Output))
						}
					}),
				}
			},
		},
		{
			Description: "Health log in inspect keeps only the latest 5 entries",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "exit 1",
					"--health-interval", "1s",
					"--health-retries", "1",
					testutil.CommonImage, "sleep", nerdtest.Infinity)
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				for i := 0; i < 7; i++ {
					helpers.Ensure("container", "healthcheck", data.Identifier())
					time.Sleep(1 * time.Second)
				}
				return helpers.Command("inspect", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(func(_ string, t tig.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						h := inspect.State.Health
						debug, _ := json.MarshalIndent(h, "", "  ")
						t.Log(string(debug))
						assert.Assert(t, h != nil, "expected health state")
						assert.Equal(t, h.Status, healthcheck.Unhealthy)
						assert.Assert(t, len(h.Log) <= 5, "expected health log to contain at most 5 entries")
					}),
				}
			},
		},
		{
			Description: "Healthcheck with large output gets truncated in health log",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "yes X | head -c 1048576", // 1MB output
					"--health-interval", "1s", "--health-timeout", "2s",
					testutil.CommonImage, "sleep", nerdtest.Infinity)
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("container", "healthcheck", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(func(_ string, t tig.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						h := inspect.State.Health
						debug, _ := json.MarshalIndent(h, "", "  ")
						t.Log(string(debug))
						assert.Assert(t, h != nil, "expected health state")
						assert.Equal(t, h.Status, healthcheck.Healthy)
						assert.Equal(t, h.FailingStreak, 0)
						assert.Assert(t, len(h.Log) == 1, "expected one log entry")
						output := h.Log[0].Output
						assert.Assert(t, strings.HasSuffix(output, "[truncated]"), "expected output to be truncated with '[truncated]'")
					}),
				}
			},
		},
		{
			Description: "Health status transitions from healthy to unhealthy after retries",
			Setup: func(data test.Data, helpers test.Helpers) {
				containerName := data.Identifier()
				helpers.Ensure("run", "-d", "--name", containerName,
					"--health-cmd", "exit 1",
					"--health-timeout", "10s",
					"--health-retries", "3",
					testutil.CommonImage, "sleep", nerdtest.Infinity)
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				for i := 0; i < 4; i++ {
					helpers.Ensure("container", "healthcheck", data.Identifier())
					time.Sleep(2 * time.Second)
				}
				return helpers.Command("inspect", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(func(stdout string, t tig.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						h := inspect.State.Health
						debug, _ := json.MarshalIndent(h, "", "  ")
						t.Log(string(debug))
						assert.Assert(t, h != nil, "expected health state")
						assert.Equal(t, h.Status, healthcheck.Unhealthy)
						assert.Assert(t, h.FailingStreak >= 3)
					}),
				}
			},
		},
		{
			Description: "Failed healthchecks in start-period do not change status",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "ls /foo || exit 1", "--health-retries", "2",
					"--health-start-period", "30s", // long enough to stay in "starting"
					testutil.CommonImage, "sleep", nerdtest.Infinity)
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				// Run healthcheck 3 times (should still be in start period)
				for i := 0; i < 3; i++ {
					helpers.Ensure("container", "healthcheck", data.Identifier())
					time.Sleep(1 * time.Second)
				}
				return helpers.Command("inspect", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(func(stdout string, t tig.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						h := inspect.State.Health
						debug, _ := json.MarshalIndent(h, "", "  ")
						t.Log(string(debug))
						assert.Assert(t, h != nil, "expected health state")
						assert.Equal(t, h.Status, healthcheck.Starting)
						assert.Equal(t, h.FailingStreak, 0, "failing streak should not increase during start period")
					}),
				}
			},
		},
		{
			Description: "Successful healthcheck in start-period sets status to healthy",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "ls || exit 1", "--health-retries", "2",
					testutil.CommonImage, "sleep", nerdtest.Infinity)
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				helpers.Ensure("container", "healthcheck", data.Identifier())
				time.Sleep(1 * time.Second)
				return helpers.Command("inspect", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(func(stdout string, t tig.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						h := inspect.State.Health
						debug, _ := json.MarshalIndent(h, "", "  ")
						t.Log(string(debug))
						assert.Assert(t, h != nil, "expected health state")
						assert.Equal(t, h.Status, healthcheck.Healthy, "expected healthy status even during start-period")
						assert.Equal(t, h.FailingStreak, 0)
					}),
				}
			},
		},
	}

	testCase.Run(t)
}

func TestHealthCheck_SystemdIntegration_Basic(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Docker)
	// Skip systemd tests in rootless environment to bypass dbus permission issues
	if rootlessutil.IsRootless() {
		t.Skip("systemd healthcheck tests are skipped in rootless environment")
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "Basic healthy container with systemd-triggered healthcheck",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "echo healthy",
					"--health-interval", "2s",
					testutil.CommonImage, "sleep", "30")
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				// Ensure proper cleanup of systemd units
				helpers.Anyhow("stop", data.Identifier())
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(func(stdout string, t tig.T) {
						var h *healthcheck.Health

						// Poll up to 5 times for health status
						maxAttempts := 5
						var finalStatus string

						for i := 0; i < maxAttempts; i++ {
							inspect := nerdtest.InspectContainer(helpers, data.Identifier())
							h = inspect.State.Health

							assert.Assert(t, h != nil, "expected health state to be present")
							finalStatus = h.Status

							// If healthy, break and pass the test
							if finalStatus == "healthy" {
								t.Log(fmt.Sprintf("Container became healthy on attempt %d/%d", i+1, maxAttempts))
								break
							}

							// If unhealthy, fail immediately
							if finalStatus == "unhealthy" {
								assert.Assert(t, false, fmt.Sprintf("Container became unhealthy on attempt %d/%d, status: %s", i+1, maxAttempts, finalStatus))
								return
							}

							// If not the last attempt, wait before retrying
							if i < maxAttempts-1 {
								t.Log(fmt.Sprintf("Attempt %d/%d: status is '%s', waiting 1 second before retry", i+1, maxAttempts, finalStatus))
								time.Sleep(1 * time.Second)
							}
						}

						if finalStatus != "healthy" {
							assert.Assert(t, false, fmt.Sprintf("Container did not become healthy after %d attempts, final status: %s", maxAttempts, finalStatus))
							return
						}

						assert.Assert(t, len(h.Log) > 0, "expected at least one health check log entry")
					}),
				}
			},
		},
		{
			Description: "Kill stops healthcheck execution and cleans up systemd timer",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "echo healthy",
					"--health-interval", "1s",
					testutil.CommonImage, "sleep", "30")
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
				helpers.Ensure("kill", data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				// Container is already killed, just remove it
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeNoCheck,
					Output: func(stdout string, t tig.T) {
						// Get container info for verification
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						containerID := inspect.ID
						h := inspect.State.Health

						// Verify health state and logs exist
						assert.Assert(t, h != nil, "expected health state to be present")
						assert.Assert(t, len(h.Log) > 0, "expected at least one health check log entry")

						// Ensure systemd timers are removed
						result := helpers.Custom("systemctl", "list-timers", "--all", "--no-pager")
						result.Run(&test.Expected{
							ExitCode: expect.ExitCodeNoCheck,
							Output: func(stdout string, _ tig.T) {
								assert.Assert(t, !strings.Contains(stdout, containerID),
									"expected nerdctl healthcheck timer for container ID %s to be removed after container stop", containerID)
							},
						})
					},
				}
			},
		},
		{
			Description: "Remove cleans up systemd timer",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "echo healthy",
					"--health-interval", "1s",
					testutil.CommonImage, "sleep", "30")
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
				helpers.Ensure("rm", "-f", data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				// Container is already removed, no cleanup needed
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeNoCheck,
					Output: func(stdout string, t tig.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						containerID := inspect.ID

						// Check systemd timers to ensure cleanup
						result := helpers.Custom("systemctl", "list-timers", "--all", "--no-pager")
						result.Run(&test.Expected{
							ExitCode: expect.ExitCodeNoCheck,
							Output: func(stdout string, _ tig.T) {
								// Verify systemd timer has been cleaned up by checking systemctl output
								// We check that no timer contains our test identifier
								assert.Assert(t, !strings.Contains(stdout, containerID),
									"expected nerdctl healthcheck timer for container ID %s to be removed after container removal", containerID)
							},
						})
					},
				}
			},
		},
		{
			Description: "Stop cleans up systemd timer",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "echo healthy",
					"--health-interval", "1s",
					testutil.CommonImage, "sleep", "30")
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
				helpers.Ensure("stop", data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				// Container is already stopped, just remove it
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeNoCheck,
					Output: func(stdout string, t tig.T) {
						// Get container info for verification
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						containerID := inspect.ID

						// Ensure systemd timers are removed
						result := helpers.Custom("systemctl", "list-timers", "--all", "--no-pager")
						result.Run(&test.Expected{
							ExitCode: expect.ExitCodeNoCheck,
							Output: func(stdout string, _ tig.T) {
								assert.Assert(t, !strings.Contains(stdout, containerID),
									"expected nerdctl healthcheck timer for container ID %s to be removed after container stop", containerID)
							},
						})
					},
				}
			},
		},
	}
	testCase.Run(t)
}

func TestHealthCheck_SystemdIntegration_Advanced(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Docker)
	// Skip systemd tests in rootless environment to bypass dbus permission issues
	if rootlessutil.IsRootless() {
		t.Skip("systemd healthcheck tests are skipped in rootless environment")
	}

	testCase.SubTests = []*test.Case{
		{
			// Tests that CreateTimer() successfully creates systemd timer units and
			// RemoveTransientHealthCheckFiles() properly cleans up units when container stops.
			Description: "Systemd timer unit creation and cleanup",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "echo healthy",
					"--health-interval", "1s",
					testutil.CommonImage, "sleep", "30")
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("inspect", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(func(stdout string, t tig.T) {
						// Get container ID and check systemd timer
						containerInspect := nerdtest.InspectContainer(helpers, data.Identifier())
						containerID := containerInspect.ID

						// Check systemd timer
						result := helpers.Custom("systemctl", "list-timers", "--all", "--no-pager")
						result.Run(&test.Expected{
							ExitCode: expect.ExitCodeNoCheck,
							Output: func(stdout string, _ tig.T) {
								// Verify that a timer exists for this specific container
								assert.Assert(t, strings.Contains(stdout, containerID),
									"expected to find nerdctl healthcheck timer containing container ID: %s", containerID)
							},
						})
						// Stop container and verify cleanup
						helpers.Ensure("stop", data.Identifier())

						// Check that timer is gone
						result = helpers.Custom("systemctl", "list-timers", "--all", "--no-pager")
						result.Run(&test.Expected{
							ExitCode: expect.ExitCodeNoCheck,
							Output: func(stdout string, _ tig.T) {
								assert.Assert(t, !strings.Contains(stdout, containerID),
									"expected nerdctl healthcheck timer for container ID %s to be removed after container stop", containerID)
							},
						})
					}),
				}
			},
		},
		{
			Description: "Container restart recreates systemd timer",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "echo restart-test",
					"--health-interval", "2s",
					testutil.CommonImage, "sleep", "60")
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				// Get container ID for verification
				containerInspect := nerdtest.InspectContainer(helpers, data.Identifier())
				containerID := containerInspect.ID

				// Step 1: Verify timer exists initially
				result := helpers.Custom("systemctl", "list-timers", "--all", "--no-pager")
				result.Run(&test.Expected{
					ExitCode: expect.ExitCodeNoCheck,
					Output: func(stdout string, t tig.T) {
						assert.Assert(t, strings.Contains(stdout, containerID),
							"expected timer for container %s to exist initially", containerID)
					},
				})

				// Step 2: Stop container
				helpers.Ensure("stop", data.Identifier())

				// Step 3: Verify timer is removed after stop
				result = helpers.Custom("systemctl", "list-timers", "--all", "--no-pager")
				result.Run(&test.Expected{
					ExitCode: expect.ExitCodeNoCheck,
					Output: func(stdout string, t tig.T) {
						assert.Assert(t, !strings.Contains(stdout, containerID),
							"expected timer for container %s to be removed after stop", containerID)
					},
				})

				// Step 4: Restart container
				helpers.Ensure("start", data.Identifier())
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())

				// Step 5: Verify timer is recreated after restart - this is our final verification
				return helpers.Custom("systemctl", "list-timers", "--all", "--no-pager")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeNoCheck,
					Output: func(stdout string, t tig.T) {
						containerInspect := nerdtest.InspectContainer(helpers, data.Identifier())
						containerID := containerInspect.ID
						assert.Assert(t, strings.Contains(stdout, containerID),
							"expected timer for container %s to be recreated after restart", containerID)
					},
				}
			},
		},
	}
	testCase.Run(t)
}
