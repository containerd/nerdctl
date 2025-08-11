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
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/healthcheck"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestContainerHealthCheckBasic(t *testing.T) {
	if rootlessutil.IsRootless() {
		t.Skip("healthcheck tests are skipped in rootless environment")
	}

	testCase := nerdtest.Setup()

	// Docker CLI does not provide a standalone healthcheck command.
	testCase.Require = require.Not(nerdtest.Docker)

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
	if rootlessutil.IsRootless() {
		t.Skip("healthcheck tests are skipped in rootless environment")
	}

	testCase := nerdtest.Setup()

	// Docker CLI does not provide a standalone healthcheck command.
	testCase.Require = require.Not(nerdtest.Docker)

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

	testCase.SubTests = []*test.Case{
		{
			Description: "Basic healthy container with systemd-triggered healthcheck",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "echo healthy",
					"--health-interval", "2s",
					testutil.CommonImage, "sleep", "30")
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
				// Wait for a healthcheck to execute
				time.Sleep(2 * time.Second)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				// Ensure proper cleanup of systemd units
				helpers.Anyhow("stop", data.Identifier())
				time.Sleep(500 * time.Millisecond) // Allow systemd cleanup
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(func(stdout string, t tig.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						h := inspect.State.Health
						assert.Assert(t, h != nil, "expected health state to be present")
						assert.Equal(t, h.Status, "healthy")
						assert.Assert(t, len(h.Log) > 0, "expected at least one health check log entry")
					}),
				}
			},
		},
		{
			Description: "Kill stops healthcheck execution",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "echo healthy",
					"--health-interval", "1s",
					testutil.CommonImage, "sleep", "30")
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
				time.Sleep(2 * time.Second)               // Wait for at least one health check to execute
				helpers.Ensure("kill", data.Identifier()) // Kill the container
				time.Sleep(3 * time.Second)               // Wait to allow any potential extra healthchecks (shouldn't happen)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				// Container is already killed, just remove it
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(func(stdout string, t tig.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						h := inspect.State.Health
						assert.Assert(t, h != nil, "expected health state to be present")
						assert.Assert(t, len(h.Log) > 0, "expected at least one health check log entry")

						// Get container FinishedAt timestamp
						containerEnd, err := time.Parse(time.RFC3339Nano, inspect.State.FinishedAt)
						assert.NilError(t, err, "parsing container FinishedAt")

						// Assert all healthcheck log start times are before container finished
						for _, entry := range h.Log {
							assert.NilError(t, err, "parsing healthcheck Start time")
							assert.Assert(t, entry.Start.Before(containerEnd), "healthcheck ran after container was killed")
						}
					}),
				}
			},
		},
	}
	testCase.Run(t)
}

// TestHealthCheck_SystemdIntegration_Advanced tests the systemd integration for container healthchecks.
//
// The systemd integration works by:
// 1. Creating systemd timer units for each container with healthchecks (CreateTimer)
// 2. Using systemd-run to schedule periodic execution of `nerdctl container healthcheck`
// 3. Managing the lifecycle of these timer units (creation, starting, stopping, cleanup)
//
// These tests verify that systemd properly manages the lifecycle and execution of healthcheck timers.
// Each test focuses on a different aspect of the integration while verifying behavior rather than
// implementation details, making them robust and environment-independent.
func TestHealthCheck_SystemdIntegration_Advanced(t *testing.T) {
	if rootlessutil.IsRootless() {
		t.Skip("systemd healthcheck tests are skipped in rootless environment")
	}

	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Docker)

	testCase.SubTests = []*test.Case{
		{
			// Tests that CreateTimer() successfully creates systemd timer units and
			// RemoveTransientHealthCheckFiles() properly cleans up units when container stops.
			// Verifies:
			// - Healthchecks execute as expected when timer is active
			// - No healthchecks run after container is stopped
			Description: "Systemd timer unit creation and cleanup",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "echo healthy",
					"--health-interval", "1s",
					testutil.CommonImage, "sleep", "30")
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
				// Wait longer for systemd timer creation and first healthcheck execution
				time.Sleep(3 * time.Second)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				// First verify healthcheck is working while container is running
				return helpers.Command("inspect", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(func(stdout string, t tig.T) {
						// Helper function to check systemctl timers
						checkSystemdTimers := func(label string) {
							t.Log("=== SYSTEMD TIMERS", label, "===")
							// Check all timers using Custom to run systemctl directly
							result := helpers.Custom("systemctl", "list-timers", "--all", "--no-pager")
							result.Run(&test.Expected{
								ExitCode: expect.ExitCodeNoCheck, // Don't care about exit code
								Output: func(stdout string, _ tig.T) {
									t.Log("All systemd timers output:")
									t.Log(stdout)
								},
							})
							if result.Stderr() != "" {
								t.Log("Systemd timers stderr:", result.Stderr())
							}

							// Check specifically for nerdctl healthcheck timers
							result = helpers.Custom("systemctl", "list-timers", "--all", "--no-pager", "*nerdctl*")
							result.Run(&test.Expected{
								ExitCode: expect.ExitCodeNoCheck, // Don't care about exit code
								Output: func(stdout string, _ tig.T) {
									t.Log("Nerdctl healthcheck timers output:")
									t.Log(stdout)
								},
							})
							if result.Stderr() != "" {
								t.Log("Nerdctl timers stderr:", result.Stderr())
							}
						}

						// Check systemd timers before starting
						checkSystemdTimers("BEFORE POLLING")

						// Retry logic to wait for health state to be populated
						var inspect dockercompat.Container
						var h *healthcheck.Health
						// Wait up to 10 seconds for health state to appear
						for i := 0; i < 10; i++ {
							inspect = nerdtest.InspectContainer(helpers, data.Identifier())
							h = inspect.State.Health

							// Log detailed state information on each attempt
							t.Log("Attempt", i+1, ": Container ID:", inspect.ID)
							t.Log("Attempt", i+1, ": Container State:", inspect.State)

							if h != nil {
								t.Log("Attempt", i+1, ": Health Status:", h.Status)
								t.Log("Attempt", i+1, ": Health FailingStreak:", h.FailingStreak)
								t.Log("Attempt", i+1, ": Health Log entries:", len(h.Log))

								if len(h.Log) > 0 {
									t.Log("Attempt", i+1, ": Latest health log entry:")
									latestEntry, _ := json.MarshalIndent(h.Log[0], "", "  ")
									t.Log(string(latestEntry))
									break
								}
							} else {
								t.Log("Attempt", i+1, ": Health state is nil")
							}

							t.Log("Health state not ready, waiting 1s...")
							time.Sleep(1 * time.Second)
						}

						// Detailed health state logging
						if h != nil {
							t.Log("=== DETAILED HEALTH STATE ===")
							t.Log("Health Status:", h.Status)
							t.Log("Failing Streak:", h.FailingStreak)
							t.Log("Number of log entries:", len(h.Log))

							// Log each health check entry with details
							for i, entry := range h.Log {
								t.Log("=== Health check entry", i, "===")
								entryJSON, _ := json.MarshalIndent(entry, "", "  ")
								t.Log(string(entryJSON))
							}
						} else {
							t.Log("=== HEALTH STATE IS NIL ===")
						}

						assert.Assert(t, h != nil, "expected health state to be present after waiting")
						assert.Assert(t, len(h.Log) > 0, "expected at least one health check log entry")
						assert.Assert(t, strings.Contains(h.Log[0].Output, "healthy"), "expected healthy output")

						// Now stop the container to test cleanup
						t.Log("=== STOPPING CONTAINER FOR CLEANUP TEST ===")
						helpers.Ensure("stop", data.Identifier())
						time.Sleep(500 * time.Millisecond) // Allow cleanup to complete

						// Verify container is stopped (this tests that systemd cleanup worked)
						inspectAfterStop := nerdtest.InspectContainer(helpers, data.Identifier())

						assert.Equal(t, "exited", inspectAfterStop.State.Status, "expected container to be stopped")
					}),
				}
			},
		},
		// {
		// 	// Tests that hcUnitName() generates unique systemd unit names for each container
		// 	// and that systemd can manage multiple concurrent healthcheck timers.
		// 	// Verifies:
		// 	// - Multiple containers can run concurrent healthcheck timers
		// 	// - Each container's healthcheck runs independently
		// 	// - No cross-contamination between container healthchecks
		// 	// This ensures the systemd integration scales properly with multiple containers.
		// 	Description: "Multiple containers with unique healthcheck behavior",
		// 	Setup: func(data test.Data, helpers test.Helpers) {
		// 		// Create two containers with healthchecks
		// 		helpers.Ensure("run", "-d", "--name", data.Identifier()+"_1",
		// 			"--health-cmd", "echo container1",
		// 			"--health-interval", "1s",
		// 			testutil.CommonImage, "sleep", "30")
		// 		helpers.Ensure("run", "-d", "--name", data.Identifier()+"_2",
		// 			"--health-cmd", "echo container2",
		// 			"--health-interval", "1s",
		// 			testutil.CommonImage, "sleep", "30")
		// 		nerdtest.EnsureContainerStarted(helpers, data.Identifier()+"_1")
		// 		nerdtest.EnsureContainerStarted(helpers, data.Identifier()+"_2")
		// 		time.Sleep(1500 * time.Millisecond) // Wait for healthchecks
		// 	},
		// 	Cleanup: func(data test.Data, helpers test.Helpers) {
		// 		// Ensure proper cleanup of systemd units for both containers
		// 		helpers.Anyhow("stop", data.Identifier()+"_1")
		// 		helpers.Anyhow("stop", data.Identifier()+"_2")
		// 		time.Sleep(500 * time.Millisecond) // Allow systemd cleanup
		// 		helpers.Anyhow("rm", "-f", data.Identifier()+"_1")
		// 		helpers.Anyhow("rm", "-f", data.Identifier()+"_2")
		// 	},
		// 	Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
		// 		// Verify both containers have healthy status
		// 		return helpers.Command("inspect", data.Identifier()+"_1")
		// 	},
		// 	Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
		// 		return &test.Expected{
		// 			ExitCode: 0,
		// 			Output: expect.All(func(stdout string, t tig.T) {
		// 				// // First, manually trigger health checks for both containers
		// 				// t.Log("Manually triggering health checks for both containers...")
		// 				// helpers.Ensure("container", "healthcheck", data.Identifier()+"_1")
		// 				// helpers.Ensure("container", "healthcheck", data.Identifier()+"_2")

		// 				// Retry logic to wait for health state to be populated for both containers
		// 				var inspect1, inspect2 dockercompat.Container
		// 				var h1, h2 *healthcheck.Health

		// 				// Wait up to 10 seconds for health state to appear for both containers
		// 				for i := 0; i < 10; i++ {
		// 					inspect1 = nerdtest.InspectContainer(helpers, data.Identifier()+"_1")
		// 					inspect2 = nerdtest.InspectContainer(helpers, data.Identifier()+"_2")
		// 					h1 = inspect1.State.Health
		// 					h2 = inspect2.State.Health
		// 					if h1 != nil && len(h1.Log) > 0 && h2 != nil && len(h2.Log) > 0 {
		// 						break
		// 					}
		// 					t.Log("Attempt", i+1, ": Health state not ready for both containers, waiting 1s...")
		// 					time.Sleep(1 * time.Second)
		// 				}

		// 				assert.Assert(t, h1 != nil, "expected health state for container 1")
		// 				assert.Assert(t, h2 != nil, "expected health state for container 2")
		// 				assert.Assert(t, len(h1.Log) > 0, "expected health log entries for container 1")
		// 				assert.Assert(t, len(h2.Log) > 0, "expected health log entries for container 2")
		// 				assert.Assert(t, strings.Contains(h1.Log[0].Output, "container1"), "expected container1 output")
		// 				assert.Assert(t, strings.Contains(h2.Log[0].Output, "container2"), "expected container2 output")
		// 			}),
		// 		}
		// 	},
		// },
		// {
		// 	// Tests that StartTimer() properly restarts the systemd timer after container restart
		// 	// and that RemoveTransientHealthCheckFiles() and CreateTimer() work together during restart.
		// 	// Verifies:
		// 	// - Healthchecks continue working after container restart
		// 	// - Timer units are properly recreated during restart process
		// 	// - No duplicate or orphaned timers are left behind
		// 	// This tests the full restart lifecycle with systemd integration.
		// 	Description: "Container restart maintains healthcheck functionality",
		// 	Setup: func(data test.Data, helpers test.Helpers) {
		// 		helpers.Ensure("run", "-d", "--name", data.Identifier(),
		// 			"--health-cmd", "echo restarted",
		// 			"--health-interval", "1s",
		// 			testutil.CommonImage, "sleep", "30")
		// 		nerdtest.EnsureContainerStarted(helpers, data.Identifier())
		// 		time.Sleep(1500 * time.Millisecond) // Wait for initial healthcheck
		// 	},
		// 	Cleanup: func(data test.Data, helpers test.Helpers) {
		// 		helpers.Anyhow("rm", "-f", data.Identifier())
		// 	},
		// 	Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
		// 		// Stop container first to ensure proper cleanup
		// 		helpers.Ensure("stop", data.Identifier())
		// 		time.Sleep(500 * time.Millisecond) // Allow cleanup to complete

		// 		// Start container again
		// 		helpers.Ensure("start", data.Identifier())
		// 		nerdtest.EnsureContainerStarted(helpers, data.Identifier())
		// 		time.Sleep(1500 * time.Millisecond) // Wait for healthcheck after restart

		// 		// Verify healthcheck still works after restart
		// 		return helpers.Command("inspect", data.Identifier())
		// 	},
		// 	Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
		// 		return &test.Expected{
		// 			ExitCode: 0,
		// 			Output: expect.All(func(stdout string, t tig.T) {
		// 				// First, manually trigger a health check to ensure it works
		// 				// t.Log("Manually triggering health check...")
		// 				// helpers.Ensure("container", "healthcheck", data.Identifier())

		// 				// Retry logic to wait for health state to be populated
		// 				var inspect dockercompat.Container
		// 				var h *healthcheck.Health

		// 				// Wait up to 10 seconds for health state to appear
		// 				for i := 0; i < 10; i++ {
		// 					inspect = nerdtest.InspectContainer(helpers, data.Identifier())
		// 					h = inspect.State.Health
		// 					if h != nil && len(h.Log) > 0 {
		// 						break
		// 					}
		// 					t.Log("Attempt", i+1, ": Health state not ready, waiting 1s...")
		// 					time.Sleep(1 * time.Second)
		// 				}

		// 				assert.Assert(t, h != nil, "expected health state to be present after waiting")
		// 				assert.Assert(t, len(h.Log) > 0, "expected health log entries after restart")
		// 				assert.Assert(t, strings.Contains(h.Log[0].Output, "restarted"), "expected restarted output")
		// 			}),
		// 		}
		// 	},
		// },
		// {
		// 	// Tests that systemd timer properties (AccuracySec=1s) work correctly
		// 	// and that timeout handling between systemd and healthcheck executor functions properly.
		// 	// Verifies:
		// 	// - Systemd respects healthcheck timeout settings
		// 	// - Failed healthchecks due to timeout are properly recorded
		// 	// - Exit code -1 indicates timeout was handled correctly
		// 	// This tests the reliability of the timer mechanism under timeout conditions.
		// 	Description: "Systemd timer handles healthcheck timeout correctly",
		// 	Setup: func(data test.Data, helpers test.Helpers) {
		// 		helpers.Ensure("run", "-d", "--name", data.Identifier(),
		// 			"--health-cmd", "sleep 5", // Command that will timeout
		// 			"--health-timeout", "1s",
		// 			"--health-interval", "1s",
		// 			testutil.CommonImage, "sleep", "30")
		// 		nerdtest.EnsureContainerStarted(helpers, data.Identifier())
		// 		time.Sleep(2 * time.Second) // Wait for timeout to occur
		// 	},
		// 	Cleanup: func(data test.Data, helpers test.Helpers) {
		// 		// Ensure proper cleanup of systemd units
		// 		helpers.Anyhow("stop", data.Identifier())
		// 		time.Sleep(500 * time.Millisecond) // Allow systemd cleanup
		// 		helpers.Anyhow("rm", "-f", data.Identifier())
		// 	},
		// 	Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
		// 		return helpers.Command("inspect", data.Identifier())
		// 	},
		// 	Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
		// 		return &test.Expected{
		// 			ExitCode: 0,
		// 			Output: expect.All(func(stdout string, t tig.T) {

		// 				// Retry logic to wait for health state to be populated
		// 				var inspect dockercompat.Container
		// 				var h *healthcheck.Health

		// 				// Wait up to 10 seconds for health state to appear
		// 				for i := 0; i < 5; i++ {
		// 					inspect = nerdtest.InspectContainer(helpers, data.Identifier())
		// 					h = inspect.State.Health
		// 					if h != nil && len(h.Log) > 0 {
		// 						break
		// 					}
		// 					t.Log("Attempt", i+1, ": Health state not ready, waiting 1s...")
		// 					time.Sleep(1 * time.Second)
		// 				}

		// 				// Log container labels for debugging
		// 				debug, _ := json.MarshalIndent(inspect.Config.Labels, "", "  ")
		// 				t.Log("Container labels:", string(debug))

		// 				debug, _ = json.MarshalIndent(h, "", "  ")
		// 				t.Log("Final health state:", string(debug))

		// 				assert.Assert(t, h != nil, "expected health state to be present after waiting")
		// 				assert.Assert(t, len(h.Log) > 0, "expected health log entries")
		// 				// Check that timeout was handled (exit code -1 indicates timeout)
		// 				assert.Equal(t, -1, h.Log[0].ExitCode, "expected timeout exit code")
		// 			}),
		// 		}
		// 	},
		// },
	}

	testCase.Run(t)
}
