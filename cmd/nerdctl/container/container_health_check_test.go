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

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/healthcheck"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestContainerHealthCheckBasic(t *testing.T) {
	testCase := nerdtest.Setup()
	testutil.DockerIncompatible(t)

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
					Output: expect.All(func(stdout, _ string, t *testing.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						h := inspect.State.Health
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
	testutil.DockerIncompatible(t)

	testCase.SubTests = []*test.Case{
		{
			Description: "Health check timeout scenario",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "sleep 10",
					"--health-timeout", "2s",
					"--health-interval", "1s",
					testutil.CommonImage, "sleep", nerdtest.Infinity)
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
					Output: expect.All(func(stdout, _ string, t *testing.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						h := inspect.State.Health
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
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				// Run healthcheck twice to ensure failing streak
				helpers.Anyhow("container", "healthcheck", data.Identifier())
				return helpers.Command("container", "healthcheck", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(func(stdout, _ string, t *testing.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						h := inspect.State.Health
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
					"--health-start-period", "5s",
					"--health-retries", "2",
					testutil.CommonImage, "sleep", nerdtest.Infinity)
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
					Output: expect.All(func(stdout, _ string, t *testing.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						h := inspect.State.Health
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
					Output: expect.All(func(stdout, _ string, t *testing.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						h := inspect.State.Health
						assert.Assert(t, h != nil, "expected health state")
						assert.Equal(t, h.Status, healthcheck.Unhealthy)
						assert.Equal(t, h.FailingStreak, 1)
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
					Output: expect.All(func(stdout, _ string, t *testing.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						h := inspect.State.Health
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
					Output: expect.All(func(stdout, _ string, t *testing.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						h := inspect.State.Health
						assert.Assert(t, h != nil, "expected health state")
						assert.Equal(t, h.Status, healthcheck.Healthy)
						assert.Equal(t, h.FailingStreak, 0)
						assert.Assert(t, strings.Contains(h.Log[0].Output, "/tmp"), "expected health log output to contain '/tmp'")
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
					Output: expect.All(func(stdout, _ string, t *testing.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						h := inspect.State.Health
						assert.Assert(t, h != nil, "expected health state")
						assert.Equal(t, h.Status, healthcheck.Unhealthy)
						assert.Assert(t, h.FailingStreak >= 3)
					}),
				}
			},
		},
		{
			Description: "Health check cmd with large output",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					"--health-cmd", "head -c 10000 /dev/urandom | base64",
					"--health-interval", "1s",
					"--health-timeout", "10s",
					testutil.CommonImage, "sleep", nerdtest.Infinity)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				for i := 0; i < 3; i++ {
					helpers.Ensure("container", "healthcheck", data.Identifier())
					time.Sleep(1 * time.Second)
				}
				return helpers.Command("inspect", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(func(stdout, _ string, t *testing.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						h := inspect.State.Health
						assert.Assert(t, h != nil, "expected health state")
						lastLog := h.Log[0].Output
						assert.Assert(t, strings.HasSuffix(lastLog, "[truncated]"), "expected truncated output with '...'")
					}),
				}
			},
		},
	}

	testCase.Run(t)
}
