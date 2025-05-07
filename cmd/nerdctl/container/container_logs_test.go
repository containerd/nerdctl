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
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestLogs(t *testing.T) {
	const expected = `foo
bar`

	testCase := nerdtest.Setup()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "--quiet", "--name", data.Identifier(), testutil.CommonImage, "sh", "-euxc", "echo foo; echo bar;")
		time.Sleep(3 * time.Second)
		data.Labels().Set("cID", data.Identifier())
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "since 1s",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("logs", "--since", "1s", data.Labels().Get("cID"))
			},
			Expected: test.Expects(0, nil, expect.DoesNotContain(expected)),
		},
		{
			Description: "since 60s",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("logs", "--since", "60s", data.Labels().Get("cID"))
			},
			// FIXME: that should be expect.Equals but rn nerdctl log adds spurious line feeds
			Expected: test.Expects(0, nil, expect.Contains(expected)),
		},
		{
			Description: "until 60s",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("logs", "--until", "60s", data.Labels().Get("cID"))
			},
			Expected: test.Expects(0, nil, expect.DoesNotContain(expected)),
		},
		{
			Description: "until 1s",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("logs", "--until", "1s", data.Labels().Get("cID"))
			},
			// FIXME: that should be expect.Equals but rn nerdctl log adds spurious line feeds
			// https://github.com/containerd/nerdctl/issues/4201
			Expected: test.Expects(0, nil, expect.Contains(expected)),
		},
		{
			Description: "follow",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("logs", "-f", data.Labels().Get("cID"))
			},
			// FIXME: that should be expect.Equals but rn nerdctl log adds spurious line feeds
			// https://github.com/containerd/nerdctl/issues/4201
			Expected: test.Expects(0, nil, expect.Contains("foo", "bar")),
		},
		{
			Description: "timestamp",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("logs", "-t", data.Labels().Get("cID"))
			},
			Expected: test.Expects(0, nil, expect.Contains(time.Now().UTC().Format("2006-01-02"))),
		},
		{
			Description: "tail flag",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("logs", "-n", "all", data.Labels().Get("cID"))
			},
			// FIXME: that should be expect.Equals but rn nerdctl log adds spurious line feeds
			// https://github.com/containerd/nerdctl/issues/4201
			Expected: test.Expects(0, nil, expect.Contains(expected)),
		},
		{
			Description: "tail flag",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("logs", "-n", "1", data.Labels().Get("cID"))
			},
			// FIXME: why?
			// Is it because of https://github.com/containerd/nerdctl/issues/4201 ?
			Expected: test.Expects(0, nil, expect.Match(regexp.MustCompile("^(?:bar\n|)$"))),
		},
	}

	testCase.Run(t)
}

// Tests whether `nerdctl logs` properly separates stdout/stderr output
// streams for containers using the jsonfile logging driver:
func TestLogsOutStreamsSeparated(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "--name", data.Identifier(), testutil.CommonImage,
			"sh", "-euc", "echo stdout1; echo stderr1 >&2; echo stdout2; echo stderr2 >&2")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("logs", data.Identifier())
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, []error{
		//revive:disable:error-strings
		errors.New("stderr1\nstderr2\n"),
	}, expect.Equals("stdout1\nstdout2\n"))

	testCase.Run(t)
}

func TestLogsWithInheritedFlags(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = require.Not(nerdtest.Docker)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("-n="+testutil.Namespace, "run", "--name", data.Identifier(), testutil.CommonImage,
			"sh", "-euxc", "echo foo; echo bar")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("-n="+testutil.Namespace, "logs", "-n", "1", data.Identifier())
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, func(stdout, info string, t *testing.T) {
		// FIXME: logs trailing spaces should be fixed and we should be able to test equals
		if !(stdout == "bar\n" || stdout == "") {
			assert.Assert(t, false, "stdout is neither empty nor bar")
		}
	})

	testCase.Run(t)
}

func TestLogsOfJournaldDriver(t *testing.T) {
	testutil.RequireExecutable(t, "journalctl")
	journalctl, _ := exec.LookPath("journalctl")
	res := icmd.RunCmd(icmd.Command(journalctl, "-xe"))
	if res.ExitCode != 0 {
		t.Skipf("current user is not allowed to access journal logs: %s", res.Combined())
	}

	t.Parallel()
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)

	defer base.Cmd("rm", containerName).Run()
	base.Cmd("run", "--network", "none", "--log-driver", "journald", "--name", containerName, testutil.CommonImage,
		"sh", "-euxc", "echo foo; echo bar").AssertOK()

	// FIXME: this is inherently flaky.
	time.Sleep(5 * time.Second)
	base.Cmd("logs", containerName).AssertOutContains("bar")
	// Run logs twice, make sure that the logs are not removed
	base.Cmd("logs", containerName).AssertOutContains("foo")

	base.Cmd("logs", "--since", "60s", containerName).AssertOutWithFunc(func(stdout string) error {
		if !strings.Contains(stdout, "bar") {
			return fmt.Errorf("expected bar, got %s", stdout)
		}
		if !strings.Contains(stdout, "foo") {
			return fmt.Errorf("expected foo, got %s", stdout)
		}
		return nil
	})

	base.Cmd("rm", "-f", containerName).AssertOK()
}

func TestLogsWithFailingContainer(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("run", "--name", data.Identifier(), testutil.CommonImage, "sh", "-euxc", "echo foo; echo bar; exit 42; echo baz")
		if runtime.GOOS == "windows" {
			// FIXME: why? @fahedouch looks like on linux this works just fine, we get the logs as soon as the container
			// exits, but not on windows.
			time.Sleep(5 * time.Second)
		}
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("logs", data.Identifier())
	}

	testCase.Expected = test.Expects(0, nil, expect.All(
		expect.Contains("bar"),
		expect.DoesNotContain("baz"),
	))

	testCase.Run(t)
}

func TestLogsWithRunningContainer(t *testing.T) {
	expected := make([]string, 10)
	for i := 0; i < 10; i++ {
		expected[i] = fmt.Sprint(i + 1)
	}

	testCase := nerdtest.Setup()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "--name", data.Identifier(), testutil.CommonImage, "sh", "-euc", "for i in `seq 1 10`; do echo $i; sleep 1; done")
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("logs", data.Identifier())
	}

	testCase.Expected = test.Expects(0, nil, expect.Contains(expected[0], expected[1:]...))

	testCase.Run(t)
}

func TestLogsWithoutNewlineOrEOF(t *testing.T) {
	testCase := nerdtest.Setup()

	// FIXME: test does not work on Windows yet because containerd doesn't send an exit event appropriately after task exit on Windows")
	// FIXME: nerdctl behavior does not match docker - test disabled for nerdctl until we fix
	testCase.Require = require.All(
		require.Linux,
		nerdtest.NerdctlNeedsFixing("https://github.com/containerd/nerdctl/issues/4201"),
	)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "--name", data.Identifier(), testutil.CommonImage, "printf", "'Hello World!\nThere is no newline'")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("logs", "-f", data.Identifier())
	}

	testCase.Expected = test.Expects(0, nil, expect.Equals("'Hello World!\nThere is no newline'"))

	testCase.Run(t)
}

func TestLogsAfterRestartingContainer(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("FIXME: test does not work on Windows yet. Restarting a container fails with: failed to create shim task: hcs::CreateComputeSystem <id>: The requested operation for attach namespace failed.: unknown")
	}

	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "--name", data.Identifier(), testutil.CommonImage,
			"printf", "'Hello World!\nThere is no newline'")
		data.Labels().Set("cID", data.Identifier())
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "logs -f works",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("logs", "-f", data.Labels().Get("cID"))
			},
			Expected: test.Expects(0, nil, expect.Contains("Hello World!", "There is no newline")),
		},
		{
			Description: "logs -f works after restart",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("start", data.Labels().Get("cID"))
				// FIXME: this is inherently flaky
				time.Sleep(3 * time.Second)
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("logs", "-f", data.Labels().Get("cID"))
			},
			Expected: test.Expects(0, nil, expect.Contains("Hello World!", "There is no newline")),
		},
	}

	testCase.Run(t)
}

func TestLogsWithForegroundContainers(t *testing.T) {
	testCase := nerdtest.Setup()
	// dual logging is not supported on Windows
	testCase.Require = require.Not(require.Windows)

	testCase.Run(t)

	testCase.SubTests = []*test.Case{
		{
			Description: "foreground",
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "--name", data.Identifier(), testutil.CommonImage, "sh", "-euxc", "echo foo; echo bar")
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("logs", data.Identifier())
			},
			Expected: test.Expects(0, nil, expect.All(
				expect.Contains("foo", "bar"),
				expect.DoesNotContain("baz"),
			)),
		},
		{
			Description: "interactive",
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-i", "--name", data.Identifier(), testutil.CommonImage, "sh", "-euxc", "echo foo; echo bar")
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("logs", data.Identifier())
			},
			Expected: test.Expects(0, nil, expect.All(
				expect.Contains("foo", "bar"),
				expect.DoesNotContain("baz"),
			)),
		},
		{
			Description: "PTY",
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Setup: func(data test.Data, helpers test.Helpers) {
				cmd := helpers.Command("run", "-t", "--name", data.Identifier(), testutil.CommonImage, "sh", "-euxc", "echo foo; echo bar")
				cmd.WithPseudoTTY()
				cmd.Run(&test.Expected{ExitCode: 0})
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("logs", data.Identifier())
			},
			Expected: test.Expects(0, nil, expect.All(
				expect.Contains("foo", "bar"),
				expect.DoesNotContain("baz"),
			)),
		},
		{
			Description: "interactivePTY",
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Setup: func(data test.Data, helpers test.Helpers) {
				cmd := helpers.Command("run", "-i", "-t", "--name", data.Identifier(), testutil.CommonImage, "sh", "-euxc", "echo foo; echo bar")
				cmd.WithPseudoTTY()
				cmd.Run(&test.Expected{ExitCode: 0})
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("logs", data.Identifier())
			},
			Expected: test.Expects(0, nil, expect.All(
				expect.Contains("foo", "bar"),
				expect.DoesNotContain("baz"),
			)),
		},
	}
}

func TestLogsTailFollowRotate(t *testing.T) {
	// FIXME this is flaky by nature... the number of lines is arbitrary, the wait is arbitrary,
	// and both are some sort of educated guess that things will mostly always kinda work maybe...
	const sampleJSONLog = `{"log":"A\n","stream":"stdout","time":"2024-04-11T12:01:09.800288974Z"}`
	const linesPerFile = 200

	testCase := nerdtest.Setup()

	// tail log is not supported on Windows
	testCase.Require = require.Not(require.Windows)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--log-driver", "json-file",
			"--log-opt", fmt.Sprintf("max-size=%d", len(sampleJSONLog)*linesPerFile),
			"--log-opt", "max-file=10",
			"--name", data.Identifier(), testutil.CommonImage,
			"sh", "-euc", "while true; do echo A; usleep 100; done")
		// FIXME: ... inherently racy...
		time.Sleep(5 * time.Second)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		cmd := helpers.Command("logs", "-f", data.Identifier())
		// FIXME: this is flaky by nature. We assume that the container has started and will output enough in 5 seconds.
		cmd.WithTimeout(5 * time.Second)
		return cmd
	}

	testCase.Expected = test.Expects(expect.ExitCodeTimeout, nil, func(stdout, info string, t *testing.T) {
		tailLogs := strings.Split(strings.TrimSpace(stdout), "\n")
		for _, line := range tailLogs {
			if line != "" {
				assert.Equal(t, "A", line)
			}
		}

		assert.Assert(t, len(tailLogs) > linesPerFile, fmt.Sprintf("expected %d lines or more, found %d", linesPerFile, len(tailLogs)))
	})

	testCase.Run(t)
}

func TestLogsNoneLoggerHasNoLogURI(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "--name", data.Identifier(), "--log-driver", "none", testutil.CommonImage, "sh", "-euxc", "echo foo")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("logs", data.Identifier())
	}

	testCase.Expected = test.Expects(1, nil, nil)

	testCase.Run(t)
}

func TestLogsWithDetails(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "--log-driver", "json-file",
			"--log-opt", "max-size=10m",
			"--log-opt", "max-file=3",
			"--log-opt", "env=ENV",
			"--env", "ENV=foo",
			"--log-opt", "labels=LABEL",
			"--label", "LABEL=bar",
			"--name", data.Identifier(), testutil.CommonImage,
			"sh", "-ec", "echo baz")
		if runtime.GOOS == "windows" {
			// FIXME: why? @fahedouch looks on linux, this works just fine, we get the logs as soon as the container
			// exits, but not on windows.
			time.Sleep(10 * time.Second)
		}
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("logs", "--details", data.Identifier())
	}

	testCase.Expected = test.Expects(0, nil, expect.Contains("ENV=foo", "LABEL=bar", "baz"))

	testCase.Run(t)
}

func TestLogsFollowNoExtraneousLineFeed(t *testing.T) {
	testCase := nerdtest.Setup()
	// This test verifies that `nerdctl logs -f` does not add extraneous line feeds
	testCase.Require = require.Not(require.Windows)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		// Create a container that outputs a message without a trailing newline
		helpers.Ensure("run", "--name", data.Identifier(), testutil.CommonImage,
			"sh", "-c", "printf 'Hello without newline'")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		// Use logs -f to follow the logs
		return helpers.Command("logs", "-f", data.Identifier())
	}

	// Verify that the output is exactly "Hello without newline" without any additional line feeds
	testCase.Expected = test.Expects(0, nil, expect.Equals("Hello without newline"))

	testCase.Run(t)
}

func TestLogsWithStartContainer(t *testing.T) {
	testCase := nerdtest.Setup()

	// Windows does not support dual logging.
	testCase.Require = require.Not(require.Windows)

	testCase.SubTests = []*test.Case{
		{
			Description: "Test logs are directed correctly for container start of a interactive container",
			Setup: func(data test.Data, helpers test.Helpers) {
				cmd := helpers.Command("run", "-it", "--name", data.Identifier(), testutil.CommonImage)
				cmd.WithPseudoTTY()
				cmd.WithFeeder(func() io.Reader {
					return strings.NewReader("echo foo\nexit\n")
				})

				cmd.Run(&test.Expected{
					ExitCode: 0,
				})

			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("start", "-ia", data.Identifier())
				cmd.WithPseudoTTY()
				cmd.WithFeeder(func() io.Reader {
					return strings.NewReader("echo bar\nexit\n")
				})
				cmd.Run(&test.Expected{
					ExitCode: 0,
				})
				cmd = helpers.Command("logs", data.Identifier())

				return cmd
			},
			Expected: test.Expects(0, nil, expect.Contains("foo", "bar")),
		},
		{
			// FIXME: is this test safe or could it be racy?
			Description: "Test logs are captured after stopping and starting a non-interactive container and continue capturing new logs",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.CommonImage, "sh", "-c", "while true; do echo foo; sleep 1; done")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				helpers.Ensure("stop", data.Identifier())
				initialLogs := helpers.Capture("logs", data.Identifier())
				initialFooCount := strings.Count(initialLogs, "foo")
				data.Labels().Set("initialFooCount", strconv.Itoa(initialFooCount))
				helpers.Ensure("start", data.Identifier())
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
				return helpers.Command("logs", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, info string, t *testing.T) {
						finalLogsCount := strings.Count(stdout, "foo")
						initialFooCount, _ := strconv.Atoi(data.Labels().Get("initialFooCount"))
						assert.Assert(t, finalLogsCount > initialFooCount, "Expected 'foo' count to increase after restart", info)
					},
				}
			},
		},
	}
	testCase.Run(t)
}
