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
	t.Parallel()
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)
	const expected = `foo
bar`

	defer base.Cmd("rm", containerName).Run()
	base.Cmd("run", "-d", "--name", containerName, testutil.CommonImage,
		"sh", "-euxc", "echo foo; echo bar").AssertOK()

	//test since / until flag
	time.Sleep(3 * time.Second)
	base.Cmd("logs", "--since", "1s", containerName).AssertOutNotContains(expected)
	base.Cmd("logs", "--since", "10s", containerName).AssertOutContains(expected)
	base.Cmd("logs", "--until", "10s", containerName).AssertOutNotContains(expected)
	base.Cmd("logs", "--until", "1s", containerName).AssertOutContains(expected)

	// Ensure follow flag works as expected:
	base.Cmd("logs", "-f", containerName).AssertOutContains("bar")
	base.Cmd("logs", "-f", containerName).AssertOutContains("foo")

	//test timestamps flag
	base.Cmd("logs", "-t", containerName).AssertOutContains(time.Now().UTC().Format("2006-01-02"))

	//test tail flag
	base.Cmd("logs", "-n", "all", containerName).AssertOutContains(expected)

	base.Cmd("logs", "-n", "1", containerName).AssertOutWithFunc(func(stdout string) error {
		if !(stdout == "bar\n" || stdout == "") {
			return fmt.Errorf("expected %q or %q, got %q", "bar", "", stdout)
		}
		return nil
	})

	base.Cmd("rm", "-f", containerName).AssertOK()
}

// Tests whether `nerdctl logs` properly separates stdout/stderr output
// streams for containers using the jsonfile logging driver:
func TestLogsOutStreamsSeparated(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.CommonImage,
			"sh", "-euc", "echo stdout1; echo stderr1 >&2; echo stdout2; echo stderr2 >&2")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		// Arbitrary, but we need to wait until the logs show up
		time.Sleep(3 * time.Second)
		return helpers.Command("logs", data.Identifier())
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, []error{
		//revive:disable:error-strings
		errors.New("stderr1\nstderr2\n"),
	}, expect.Equals("stdout1\nstdout2\n"))

	testCase.Run(t)
}

func TestLogsWithInheritedFlags(t *testing.T) {
	// Seen flaky with Docker
	t.Parallel()
	base := testutil.NewBase(t)
	for k, v := range base.Args {
		if strings.HasPrefix(v, "--namespace=") {
			base.Args[k] = "-n=" + testutil.Namespace
		}
	}
	containerName := testutil.Identifier(t)

	defer base.Cmd("rm", containerName).Run()
	base.Cmd("run", "-d", "--name", containerName, testutil.CommonImage,
		"sh", "-euxc", "echo foo; echo bar").AssertOK()

	// It appears this test flakes out with Docker seeing only "foo\n"
	// Tentatively adding a pause in case this is just slow
	time.Sleep(time.Second)
	// test rootCmd alias `-n` already used in logs subcommand
	base.Cmd("logs", "-n", "1", containerName).AssertOutWithFunc(func(stdout string) error {
		if !(stdout == "bar\n" || stdout == "") {
			return fmt.Errorf("expected %q or %q, got %q", "bar", "", stdout)
		}
		return nil
	})
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
	base.Cmd("run", "-d", "--network", "none", "--log-driver", "journald", "--name", containerName, testutil.CommonImage,
		"sh", "-euxc", "echo foo; echo bar").AssertOK()

	time.Sleep(3 * time.Second)
	base.Cmd("logs", containerName).AssertOutContains("bar")
	// Run logs twice, make sure that the logs are not removed
	base.Cmd("logs", containerName).AssertOutContains("foo")

	base.Cmd("logs", "--since", "5s", containerName).AssertOutWithFunc(func(stdout string) error {
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
	t.Parallel()
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)
	defer base.Cmd("rm", containerName).Run()
	base.Cmd("run", "-d", "--name", containerName, testutil.CommonImage,
		"sh", "-euxc", "echo foo; echo bar; exit 42; echo baz").AssertOK()
	time.Sleep(3 * time.Second)
	// AssertOutContains also asserts that the exit code of the logs command == 0,
	// even when the container is failing
	base.Cmd("logs", "-f", containerName).AssertOutContains("bar")
	base.Cmd("logs", "-f", containerName).AssertOutNotContains("baz")
	base.Cmd("rm", "-f", containerName).AssertOK()
}

func TestLogsWithRunningContainer(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)
	defer base.Cmd("rm", "-f", containerName).Run()
	expected := make([]string, 10)
	for i := 0; i < 10; i++ {
		expected[i] = fmt.Sprint(i + 1)
	}

	base.Cmd("run", "-d", "--name", containerName, testutil.CommonImage,
		"sh", "-euc", "for i in `seq 1 10`; do echo $i; sleep 1; done").AssertOK()
	base.Cmd("logs", "-f", containerName).AssertOutContainsAll(expected...)
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
		helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.CommonImage, "printf", "'Hello World!\nThere is no newline'")
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		// FIXME: arbitrary timeouts are by nature a problem.
		time.Sleep(5 * time.Second)
		return helpers.Command("logs", "-f", data.Identifier())
	}
	testCase.Expected = test.Expects(0, nil, expect.Equals("'Hello World!\nThere is no newline'"))
	testCase.Run(t)
}

func TestLogsAfterRestartingContainer(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("FIXME: test does not work on Windows yet. Restarting a container fails with: failed to create shim task: hcs::CreateComputeSystem <id>: The requested operation for attach namespace failed.: unknown")
	}
	t.Parallel()
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)
	defer base.Cmd("rm", "-f", containerName).Run()
	base.Cmd("run", "-d", "--name", containerName, testutil.CommonImage,
		"printf", "'Hello World!\nThere is no newline'").AssertOK()
	expected := []string{"Hello World!", "There is no newline"}
	time.Sleep(3 * time.Second)
	base.Cmd("logs", "-f", containerName).AssertOutContainsAll(expected...)
	// restart and check logs again
	base.Cmd("start", containerName)
	time.Sleep(3 * time.Second)
	base.Cmd("logs", "-f", containerName).AssertOutContainsAll(expected...)
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

func TestTailFollowRotateLogs(t *testing.T) {
	// FIXME this is flaky by nature... 2 lines is arbitrary, 10000 ms is arbitrary, and both are some sort of educated
	// guess that things will mostly always kinda work maybe...
	// Furthermore, parallelizing will put pressure on the daemon which might be even slower in answering, increasing
	// the risk of transient failure.
	// This test needs to be rethought entirely
	// t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("tail log is not supported on Windows")
	}
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)

	const sampleJSONLog = `{"log":"A\n","stream":"stdout","time":"2024-04-11T12:01:09.800288974Z"}`
	const linesPerFile = 200

	defer base.Cmd("rm", "-f", containerName).Run()
	base.Cmd("run", "-d", "--log-driver", "json-file",
		"--log-opt", fmt.Sprintf("max-size=%d", len(sampleJSONLog)*linesPerFile),
		"--log-opt", "max-file=10",
		"--name", containerName, testutil.CommonImage,
		"sh", "-euc", "while true; do echo A; usleep 100; done").AssertOK()

	tailLogCmd := base.Cmd("logs", "-f", containerName)
	tailLogCmd.Timeout = 1000 * time.Millisecond
	logRun := tailLogCmd.Run()
	tailLogs := strings.Split(strings.TrimSpace(logRun.Stdout()), "\n")
	for _, line := range tailLogs {
		if line != "" {
			assert.Equal(t, "A", line)
		}
	}
	assert.Equal(t, true, len(tailLogs) > linesPerFile, logRun.Stderr())
}
func TestNoneLoggerHasNoLogURI(t *testing.T) {
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
		helpers.Ensure("run", "-d", "--log-driver", "json-file",
			"--log-opt", "max-size=10m",
			"--log-opt", "max-file=3",
			"--log-opt", "env=ENV",
			"--env", "ENV=foo",
			"--log-opt", "labels=LABEL",
			"--label", "LABEL=bar",
			"--name", data.Identifier(), testutil.CommonImage,
			"sh", "-ec", "echo baz")
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
		// and then sleeps to keep the container running for the logs -f command
		helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.CommonImage,
			"sh", "-c", "printf 'Hello without newline'; sleep 5")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		// Use logs -f to follow the logs
		// The container will exit after 5 seconds, so we don't need an explicit timeout
		// Arbitrary, but we need to wait until the logs show up
		time.Sleep(3 * time.Second)
		return helpers.Command("logs", "-f", data.Identifier())
	}

	// Verify that the output is exactly "Hello without newline" without any additional line feeds
	testCase.Expected = test.Expects(0, nil, expect.Equals("Hello without newline"))

	testCase.Run(t)
}

func TestLogsWithStartContainer(t *testing.T) {
	testCase := nerdtest.Setup()

	// For windows we  havent added support for dual logging so not adding the test.
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
