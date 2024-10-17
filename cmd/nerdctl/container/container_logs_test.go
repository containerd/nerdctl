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
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
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
	base.Cmd("logs", "-t", containerName).AssertOutContains(time.Now().Format("2006-01-02"))

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
	t.Parallel()
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)

	defer base.Cmd("rm", containerName).Run()
	base.Cmd("run", "-d", "--name", containerName, testutil.CommonImage,
		"sh", "-euc", "echo stdout1; echo stderr1 >&2; echo stdout2; echo stderr2 >&2").AssertOK()
	time.Sleep(3 * time.Second)

	base.Cmd("logs", containerName).AssertOutStreamsExactly("stdout1\nstdout2\n", "stderr1\nstderr2\n")
}

func TestLogsWithInheritedFlags(t *testing.T) {
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
	t.Parallel()
	testutil.RequireExecutable(t, "journalctl")
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

func TestLogsWithForegroundContainers(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("dual logging is not supported on Windows")
	}
	base := testutil.NewBase(t)
	tid := testutil.Identifier(t)

	// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
	// unbuffer(1) can be installed with `apt-get install expect`.
	unbuffer := []string{"unbuffer"}

	testCases := []struct {
		name  string
		flags []string
		tty   bool
	}{
		{
			name:  "foreground",
			flags: nil,
			tty:   false,
		},
		{
			name:  "interactive",
			flags: []string{"-i"},
			tty:   false,
		},
		{
			name:  "PTY",
			flags: []string{"-t"},
			tty:   true,
		},
		{
			name:  "interactivePTY",
			flags: []string{"-i", "-t"},
			tty:   true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		func(t *testing.T) {
			containerName := tid + "-" + tc.name
			var cmdArgs []string
			defer base.Cmd("rm", "-f", containerName).Run()
			cmdArgs = append(cmdArgs, "run", "--name", containerName)
			cmdArgs = append(cmdArgs, tc.flags...)
			cmdArgs = append(cmdArgs, testutil.CommonImage, "sh", "-euxc", "echo foo; echo bar")

			if tc.tty {
				base.CmdWithHelper(unbuffer, cmdArgs...).AssertOK()
			} else {
				base.Cmd(cmdArgs...).AssertOK()
			}

			base.Cmd("logs", containerName).AssertOutContains("foo")
			base.Cmd("logs", containerName).AssertOutContains("bar")
			base.Cmd("logs", containerName).AssertOutNotContains("baz")
		}(t)
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
