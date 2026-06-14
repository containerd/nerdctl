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
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestRunEntrypointWithBuild(t *testing.T) {
	nerdtest.Setup()

	dockerfile := fmt.Sprintf(`FROM %s
ENTRYPOINT ["echo", "foo"]
CMD ["echo", "bar"]
	`, testutil.CommonImage)

	testCase := &test.Case{
		Require: nerdtest.Build,
		Setup: func(data test.Data, helpers test.Helpers) {
			data.Temp().Save(dockerfile, "Dockerfile")
			data.Labels().Set("image", data.Identifier())
			helpers.Ensure("build", "-t", data.Labels().Get("image"), data.Temp().Path())
		},
		SubTests: []*test.Case{
			{
				Description: "Run image",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", data.Labels().Get("image"))
				},
				Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("foo echo bar\n")),
			},
			{
				Description: "Run image empty entrypoint",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", "--entrypoint", "", data.Labels().Get("image"))
				},
				Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
			},
			{
				Description: "Run image time entrypoint",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", "--entrypoint", "time", data.Labels().Get("image"))
				},
				Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
			},
			{
				Description: "Run image empty entrypoint custom command",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", "--entrypoint", "", data.Labels().Get("image"), "echo", "blah")
				},
				Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.All(
					expect.Contains("blah"),
					expect.DoesNotContain("foo", "bar"),
				)),
			},
			{
				Description: "Run image time entrypoint custom command",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", "--entrypoint", "time", data.Labels().Get("image"), "echo", "blah")
				},
				Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.All(
					expect.Contains("blah"),
					expect.DoesNotContain("foo", "bar"),
				)),
			},
		},
	}

	testCase.Run(t)
}

func TestRunWorkdir(t *testing.T) {
	testCase := nerdtest.Setup()

	dir := "/foo"
	if runtime.GOOS == "windows" {
		dir = "c:" + dir
	}

	testCase.Command = test.Command("run", "--rm", "--workdir="+dir, testutil.CommonImage, "pwd")

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains(dir))

	testCase.Run(t)
}

func TestRunWithDoubleDash(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = require.Not(nerdtest.Docker)

	testCase.Command = test.Command("run", "--rm", testutil.CommonImage, "--", "sh", "-euxc", "exit 0")

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, nil)

	testCase.Run(t)
}

func TestRunExitCode(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier("exit0"))
		helpers.Anyhow("rm", "-f", data.Identifier("exit123"))
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "--name", data.Identifier("exit0"), testutil.CommonImage, "sh", "-euxc", "exit 0")
		helpers.Command("run", "--name", data.Identifier("exit123"), testutil.CommonImage, "sh", "-euxc", "exit 123").
			Run(&test.Expected{ExitCode: 123})
	}

	testCase.Command = test.Command("ps", "-a")

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Errors:   nil,
			Output: expect.All(
				expect.Match(regexp.MustCompile("Exited [(]123[)][A-Za-z0-9 ]+"+data.Identifier("exit123"))),
				expect.Match(regexp.MustCompile("Exited [(]0[)][A-Za-z0-9 ]+"+data.Identifier("exit0"))),
				func(stdout string, t tig.T) {
					assert.Equal(t, nerdtest.InspectContainer(helpers, data.Identifier("exit0")).State.Status, "exited")
					assert.Equal(t, nerdtest.InspectContainer(helpers, data.Identifier("exit123")).State.Status, "exited")
				},
			),
		}
	}

	testCase.Run(t)
}

func TestRunCIDFile(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "--rm", "--cidfile", data.Temp().Path("cid-file"), testutil.CommonImage)
		data.Temp().Exists("cid-file")
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "--rm", "--cidfile", data.Temp().Path("cid-file"), testutil.CommonImage)
	}

	// Docker will return 125 while nerdctl returns 1, so, generic fail instead of specific exit code
	testCase.Expected = test.Expects(expect.ExitCodeGenericFail, []error{errors.New("container ID file found")}, nil)

	testCase.Run(t)
}

func TestRunEnvFile(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Env = map[string]string{
		"HOST_ENV": "ENV-IN-HOST",
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Temp().Save("# this is a comment line\nTESTKEY1=TESTVAL1", "env1-file")
		data.Temp().Save("# this is a comment line\nTESTKEY2=TESTVAL2\nHOST_ENV", "env2-file")
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command(
			"run", "--rm",
			"--env-file", data.Temp().Path("env1-file"),
			"--env-file", data.Temp().Path("env2-file"),
			testutil.CommonImage, "env")
	}

	testCase.Expected = test.Expects(
		expect.ExitCodeSuccess,
		nil,
		expect.Contains("TESTKEY1=TESTVAL1", "TESTKEY2=TESTVAL2", "HOST_ENV=ENV-IN-HOST"),
	)

	testCase.Run(t)
}

func TestRunEnv(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Env = map[string]string{
		"CORGE":  "corge-value-in-host",
		"GARPLY": "garply-value-in-host",
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "--rm",
			"--env", "FOO=foo1,foo2",
			"--env", "BAR=bar1 bar2",
			"--env", "BAZ=",
			"--env", "QUX", // not exported in OS
			"--env", "QUUX=quux1",
			"--env", "QUUX=quux2",
			"--env", "CORGE", // OS exported
			"--env", "GRAULT=grault_key=grault_value", // value contains `=` char
			"--env", "GARPLY=", // OS exported
			"--env", "WALDO=", // not exported in OS
			testutil.CommonImage, "env")
	}

	validate := []test.Comparator{
		expect.Contains(
			"\nFOO=foo1,foo2\n",
			"\nBAR=bar1 bar2\n",
			"\nQUUX=quux2\n",
			"\nCORGE=corge-value-in-host\n",
			"\nGRAULT=grault_key=grault_value\n",
		),
		expect.DoesNotContain("QUX"),
	}

	if runtime.GOOS != "windows" {
		validate = append(
			validate,
			expect.Contains(
				"\nBAZ=\n",
				"\nGARPLY=\n",
				"\nWALDO=\n",
			),
		)
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.All(validate...))

	testCase.Run(t)
}

func TestRunHostnameEnv(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.SubTests = []*test.Case{
		{
			Description: "default hostname",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("run", "--rm", "--quiet", testutil.CommonImage)
				// Note: on Windows, just straight passing the command will not work (some cmd escaping weirdness?)
				cmd.Feed(strings.NewReader(`[[ "HOSTNAME=$(hostname)" == "$(env | grep HOSTNAME)" ]]`))
				return cmd
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
		{
			Description: "with --hostname",
			// Windows does not support --hostname
			Require:  require.Not(require.Windows),
			Command:  test.Command("run", "--rm", "--quiet", "--hostname", "foobar", testutil.CommonImage, "env"),
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("HOSTNAME=foobar")),
		},
	}

	testCase.Run(t)
}

func TestRunStdin(t *testing.T) {
	testCase := nerdtest.Setup()

	const testStr = "test-run-stdin"

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		cmd := helpers.Command("run", "--rm", "-i", testutil.CommonImage, "cat")
		cmd.Feed(strings.NewReader(testStr))
		return cmd
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Equals(testStr))

	testCase.Run(t)
}

func TestRunWithJsonFileLogDriver(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(require.Windows)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--log-driver", "json-file", "--log-opt", "max-size=5K", "--log-opt", "max-file=2",
			"--name", data.Identifier(), testutil.CommonImage,
			"sh", "-euxc", "hexdump -C /dev/urandom | head -n1000")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		time.Sleep(3 * time.Second)
		return helpers.Command("inspect", data.Identifier())
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, t tig.T) {
				inspect := nerdtest.InspectContainer(helpers, data.Identifier())
				logJSONPath := filepath.Dir(inspect.LogPath)
				// matches = current log file + old log files to retain
				matches, err := filepath.Glob(filepath.Join(logJSONPath, inspect.ID+"*"))
				assert.NilError(t, err)
				assert.Equal(t, len(matches), 2, "the number of log files is not equal to 2 files, got: %v", matches)
				for _, file := range matches {
					fInfo, err := os.Stat(file)
					assert.NilError(t, err)
					// The log file size is compared to 5200 bytes (instead 5k) to keep docker compatibility.
					// Docker log rotation lacks precision because the size check is done at the log entry level
					// and not at the byte level (io.Writer), so docker log files can exceed 5k
					assert.Assert(t, fInfo.Size() <= 5200, "file size exceeded 5k: %s", file)
				}
			},
		}
	}

	testCase.Run(t)
}

func TestRunWithJsonFileLogDriverAndLogPathOpt(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.All(require.Not(require.Windows), require.Not(nerdtest.Docker))

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		customLogJSONPath := filepath.Join(data.Temp().Path(), data.Identifier(), data.Identifier()+"-json.log")
		data.Labels().Set("logPath", customLogJSONPath)
		helpers.Ensure("run", "-d", "--log-driver", "json-file",
			"--log-opt", fmt.Sprintf("log-path=%s", customLogJSONPath),
			"--log-opt", "max-size=5K", "--log-opt", "max-file=2",
			"--name", data.Identifier(), testutil.CommonImage,
			"sh", "-euxc", "hexdump -C /dev/urandom | head -n1000")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		time.Sleep(3 * time.Second)
		return helpers.Command("inspect", data.Identifier())
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, t tig.T) {
				customLogJSONPath := data.Labels().Get("logPath")
				rawBytes, err := os.ReadFile(customLogJSONPath)
				assert.NilError(t, err)
				assert.Assert(t, len(rawBytes) > 0, "logs are not written correctly to log-path: %s", customLogJSONPath)
				// matches = current log file + old log files to retain
				matches, err := filepath.Glob(filepath.Join(filepath.Dir(customLogJSONPath), data.Identifier()+"*"))
				assert.NilError(t, err)
				assert.Equal(t, len(matches), 2, "the number of log files is not equal to 2 files, got: %v", matches)
				for _, file := range matches {
					fInfo, err := os.Stat(file)
					assert.NilError(t, err)
					assert.Assert(t, fInfo.Size() <= 5200, "file size exceeded 5k: %s", file)
				}
			},
		}
	}

	testCase.Run(t)
}

func waitForJournaldLogs(since, filter string, expected ...string) {
	journalctl, _ := exec.LookPath("journalctl")
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		res := icmd.RunCmd(icmd.Command(journalctl, "--no-pager", "--since", since, filter))
		found := true
		for _, s := range expected {
			if !strings.Contains(res.Stdout(), s) {
				found = false
				break
			}
		}
		if found {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func journaldRequire() *test.Requirement {
	return require.All(
		require.Not(require.Windows),
		require.Binary("journalctl"),
		&test.Requirement{
			Check: func(data test.Data, helpers test.Helpers) (bool, string) {
				journalctl, _ := exec.LookPath("journalctl")
				res := icmd.RunCmd(icmd.Command(journalctl, "-xe"))
				if res.ExitCode != expect.ExitCodeSuccess {
					return false, fmt.Sprintf("current user is not allowed to access journal logs: %s", res.Combined())
				}
				return true, "journald is accessible"
			},
		},
	)
}

func journaldExpected() func(data test.Data, helpers test.Helpers) *test.Expected {
	return func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: expect.All(
				expect.Contains("foo"),
				expect.Contains("bar"),
			),
		}
	}
}

func TestRunWithJournaldLogDriver(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = journaldRequire()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		startTime := time.Now().Format("2006-01-02 15:04:05")
		helpers.Ensure("run", "-d", "--log-driver", "journald", "--name", data.Identifier(), testutil.CommonImage,
			"sh", "-euxc", "echo foo; echo bar")
		inspect := nerdtest.InspectContainer(helpers, data.Identifier())
		data.Labels().Set("startTime", startTime)
		data.Labels().Set("shortID", inspect.ID[:12])
		data.Labels().Set("containerName", data.Identifier())
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	type journaldTC struct {
		description string
		filter      func(data test.Data) string
	}

	tcs := []journaldTC{
		{
			description: "filter journald logs using SYSLOG_IDENTIFIER field",
			filter:      func(data test.Data) string { return fmt.Sprintf("SYSLOG_IDENTIFIER=%s", data.Labels().Get("shortID")) },
		},
		{
			description: "filter journald logs using CONTAINER_NAME field",
			filter: func(data test.Data) string {
				return fmt.Sprintf("CONTAINER_NAME=%s", data.Labels().Get("containerName"))
			},
		},
		{
			description: "filter journald logs using IMAGE_NAME field",
			filter:      func(data test.Data) string { return fmt.Sprintf("IMAGE_NAME=%s", testutil.CommonImage) },
		},
	}

	for _, tc := range tcs {
		testCase.SubTests = append(testCase.SubTests, &test.Case{
			Description: tc.description,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				journalctl, _ := exec.LookPath("journalctl")
				filter := tc.filter(data)
				since := data.Labels().Get("startTime")
				waitForJournaldLogs(since, filter, "foo", "bar")
				return helpers.Custom(journalctl, "--no-pager", "--since", since, filter)
			},
			Expected: journaldExpected(),
		})
	}

	testCase.Run(t)
}

func TestRunWithJournaldLogDriverAndLogOpt(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = journaldRequire()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		startTime := time.Now().Format("2006-01-02 15:04:05")
		helpers.Ensure("run", "-d", "--log-driver", "journald", "--log-opt", "tag={{.FullID}}", "--name", data.Identifier(), testutil.CommonImage,
			"sh", "-euxc", "echo foo; echo bar")
		inspect := nerdtest.InspectContainer(helpers, data.Identifier())
		data.Labels().Set("startTime", startTime)
		data.Labels().Set("fullID", inspect.ID)
		waitForJournaldLogs(startTime, fmt.Sprintf("SYSLOG_IDENTIFIER=%s", inspect.ID), "foo", "bar")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		journalctl, _ := exec.LookPath("journalctl")
		return helpers.Custom(journalctl, "--no-pager", "--since", data.Labels().Get("startTime"),
			fmt.Sprintf("SYSLOG_IDENTIFIER=%s", data.Labels().Get("fullID")))
	}

	testCase.Expected = journaldExpected()

	testCase.Run(t)
}

func TestRunWithLogBinary(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.All(
		nerdtest.Build,
		require.Not(require.Windows),
		require.Not(nerdtest.Docker),
	)

	var dockerfile = `
FROM ` + testutil.GolangImage + ` as builder
WORKDIR /go/src/
RUN mkdir -p logger
WORKDIR /go/src/logger
RUN echo '\
	package main \n\
	\n\
	import ( \n\
	"bufio" \n\
	"context" \n\
	"fmt" \n\
	"io" \n\
	"os" \n\
	"path/filepath" \n\
	"sync" \n\
	\n\
	"github.com/containerd/containerd/v2/core/runtime/v2/logging"\n\
	)\n\

	func main() {\n\
		logging.Run(log)\n\
	}\n\

	func log(ctx context.Context, config *logging.Config, ready func() error) error {\n\
		var wg sync.WaitGroup \n\
		wg.Add(2) \n\
		// forward both stdout and stderr to temp files \n\
		go copy(&wg, config.Stdout, config.ID, "stdout") \n\
		go copy(&wg, config.Stderr, config.ID, "stderr") \n\

		// signal that we are ready and setup for the container to be started \n\
		if err := ready(); err != nil { \n\
		return err \n\
		} \n\
		wg.Wait() \n\
		return nil \n\
	}\n\
	\n\
	func copy(wg *sync.WaitGroup, r io.Reader, id string, kind string) { \n\
		f, _ := os.Create(filepath.Join(os.TempDir(), fmt.Sprintf("%s_%s.log", id, kind))) \n\
		defer f.Close() \n\
		defer wg.Done() \n\
		s := bufio.NewScanner(r) \n\
		for s.Scan() { \n\
			f.WriteString(s.Text()) \n\
		} \n\
	}\n' >> main.go


RUN go mod init
# Workaround for "package slices is not in GOROOT" https://github.com/containerd/nerdctl/issues/4214
RUN go get github.com/containerd/containerd/v2@v2.0.5
RUN go mod tidy
RUN go build .

FROM scratch
COPY --from=builder /go/src/logger/logger /
	`

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Temp().Save(dockerfile, "Dockerfile")
		helpers.Ensure("build", data.Temp().Path(),
			"--output", fmt.Sprintf("type=local,src=/go/src/logger/logger,dest=%s", data.Temp().Path()))
		helpers.Anyhow("container", "rm", "-f", data.Identifier())
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("container", "rm", "-f", data.Identifier())
		helpers.Anyhow("builder", "prune", "--all", "--force")
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "-d",
			"--log-driver", fmt.Sprintf("binary://%s/logger", data.Temp().Path()),
			"--name", data.Identifier(), testutil.CommonImage,
			"sh", "-euxc", "echo foo; echo bar")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, t tig.T) {
				containerID := strings.TrimSpace(stdout)
				logBytes, err := os.ReadFile(filepath.Join(os.TempDir(),
					fmt.Sprintf("%s_stdout.log", containerID)))
				assert.NilError(t, err)
				log := string(logBytes)
				assert.Assert(t, strings.Contains(log, "foo"))
				assert.Assert(t, strings.Contains(log, "bar"))
			},
		}
	}

	testCase.Run(t)
}

// history: There was a bug that the --add-host items disappear when the another container created.
// This test ensures that it doesn't happen.
// (https://github.com/containerd/nerdctl/issues/2560)
func TestRunAddHostRemainsWhenAnotherContainerCreated(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(require.Windows)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--add-host", "test-add-host:10.0.0.1", "--name", data.Identifier(), testutil.CommonImage, "sleep", nerdtest.Infinity)
		helpers.Ensure("exec", data.Identifier(), "grep", "10.0.0.1.*test-add-host", "/etc/hosts")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("container", "rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		// run another container to verify --add-host entry is not disturbed
		helpers.Ensure("run", "--rm", testutil.CommonImage)
		return helpers.Command("exec", data.Identifier(), "cat", "/etc/hosts")
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil,
		expect.Match(regexp.MustCompile(`(?m)^10\.0\.0\.1\s+test-add-host$`)),
	)

	testCase.Run(t)
}

// https://github.com/containerd/nerdctl/issues/2726
func TestRunRmTime(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("pull", "--quiet", testutil.CommonImage)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		data.Labels().Set("start", time.Now().Format(time.RFC3339Nano))
		return helpers.Command("run", "--rm", testutil.CommonImage, "true")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, t tig.T) {
				start, _ := time.Parse(time.RFC3339Nano, data.Labels().Get("start"))
				took := time.Since(start)
				deadline := 3 * time.Second
				// FIXME: Investigate? it appears that since the move to containerd 2 on Windows, this is taking longer.
				if runtime.GOOS == "windows" {
					deadline = 10 * time.Second
				}
				assert.Assert(t, took <= deadline, "expected to have completed in %v, took %v", deadline, took)
			},
		}
	}

	testCase.Run(t)
}

func TestRunAttachFlag(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(require.Windows)

	type attachTC struct {
		description string
		args        []string
		useStdin    bool
		isError     bool
		testStr     string
		expectedOut string
		dockerOut   string
	}

	tcs := []attachTC{
		{
			description: "AttachFlagStdin",
			args:        []string{"-a", "STDIN", "-a", "STDOUT"},
			useStdin:    true,
			testStr:     "test-run-stdio",
			expectedOut: "test-run-stdio",
			dockerOut:   "test-run-stdio",
		},
		{
			description: "AttachFlagStdOut",
			args:        []string{"-a", "STDOUT"},
			testStr:     "foo",
			expectedOut: "foo",
			dockerOut:   "foo",
		},
		{
			description: "AttachFlagMixedValue",
			args:        []string{"-a", "STDIN", "-a", "invalid-value"},
			isError:     true,
			testStr:     "foo",
			expectedOut: "invalid stream specified with -a flag. Valid streams are STDIN, STDOUT, and STDERR",
			dockerOut:   "valid streams are STDIN, STDOUT and STDERR",
		},
		{
			description: "AttachFlagInvalidValue",
			args:        []string{"-a", "invalid-stream"},
			isError:     true,
			testStr:     "foo",
			expectedOut: "invalid stream specified with -a flag. Valid streams are STDIN, STDOUT, and STDERR",
			dockerOut:   "valid streams are STDIN, STDOUT and STDERR",
		},
		{
			description: "AttachFlagCaseInsensitive",
			args:        []string{"-a", "stdin", "-a", "stdout"},
			useStdin:    true,
			testStr:     "test-run-stdio",
			expectedOut: "test-run-stdio",
			dockerOut:   "test-run-stdio",
		},
	}

	for _, tc := range tcs {
		testCase.SubTests = append(testCase.SubTests, &test.Case{
			Description: tc.description,
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				var args []string
				if tc.useStdin {
					args = append([]string{"run", "--rm", "-i"}, tc.args...)
				} else {
					args = append([]string{"run"}, tc.args...)
				}
				args = append(args, "--name", data.Identifier(), testutil.CommonImage)
				if !tc.useStdin {
					args = append(args, "sh", "-euxc", "echo "+tc.testStr)
				}
				cmd := helpers.Command(args...)
				if tc.useStdin {
					cmd.Feed(strings.NewReader("echo " + tc.testStr + "\nexit\n"))
				}
				return cmd
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				out := tc.expectedOut
				if nerdtest.IsDocker() {
					out = tc.dockerOut
				}
				if tc.isError {
					return &test.Expected{
						ExitCode: expect.ExitCodeGenericFail,
						Errors:   []error{errors.New(out)},
					}
				}
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output:   expect.Contains(out),
				}
			},
		})
	}

	testCase.Run(t)
}

func TestRunQuiet(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rmi", "-f", testutil.CommonImage)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rmi", "-f", testutil.CommonImage)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "--rm", "--quiet", testutil.CommonImage, "echo", "test run quiet")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		// Docker and nerdctl image pulls are not 1:1.
		pullSentinel := "resolved"
		if nerdtest.IsDocker() {
			pullSentinel = "Pull complete"
		}
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: expect.All(
				expect.Contains("test run quiet"),
				expect.DoesNotContain(pullSentinel),
			),
		}
	}

	testCase.Run(t)
}

func TestRunFromOCIArchive(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.All(nerdtest.Build, require.Not(nerdtest.Docker))

	const sentinel = "test-nerdctl-run-from-oci-archive"

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		tag := fmt.Sprintf("%s:latest", data.Identifier())
		helpers.Anyhow("rmi", "-f", tag)

		dockerfile := fmt.Sprintf("FROM %s\nCMD [\"echo\", \"%s\"]", testutil.CommonImage, sentinel)
		data.Temp().Save(dockerfile, "Dockerfile")
		tarPath := data.Temp().Path(data.Identifier() + ".tar")
		helpers.Ensure("build", "--tag", tag, fmt.Sprintf("--output=type=oci,dest=%s", tarPath), data.Temp().Path())
		data.Labels().Set("tag", tag)
		data.Labels().Set("tarPath", tarPath)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rmi", "-f", data.Labels().Get("tag"))
		helpers.Anyhow("builder", "prune", "--all", "--force")
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "--rm", fmt.Sprintf("oci-archive://%s", data.Labels().Get("tarPath")))
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains(sentinel))

	testCase.Run(t)
}

func TestRunDomainname(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(require.Windows)

	type domainnameTC struct {
		description string
		hostname    string
		domainname  string
		cmd         string
		cmdFlag     string
		expectedOut string
	}

	tcs := []domainnameTC{
		{
			description: "Check domain name",
			hostname:    "foobar",
			domainname:  "example.com",
			cmd:         "hostname",
			cmdFlag:     "-d",
			expectedOut: "example.com",
		},
		{
			description: "check fqdn",
			hostname:    "foobar",
			domainname:  "example.com",
			cmd:         "hostname",
			cmdFlag:     "-f",
			expectedOut: "foobar.example.com",
		},
	}

	for _, tc := range tcs {
		testCase.SubTests = append(testCase.SubTests, &test.Case{
			Description: tc.description,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm",
					"--hostname", tc.hostname,
					"--domainname", tc.domainname,
					testutil.CommonImage,
					tc.cmd, tc.cmdFlag,
				)
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains(tc.expectedOut)),
		})
	}

	testCase.Run(t)
}

func TestRunHealthcheckFlags(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Rootless)

	testCases := []struct {
		name              string
		args              []string
		shouldFail        bool
		expectTest        []string
		expectRetries     int
		expectInterval    time.Duration
		expectTimeout     time.Duration
		expectStartPeriod time.Duration
	}{
		{
			name: "Valid_full_config",
			args: []string{
				"--health-cmd", "curl -f http://localhost || exit 1",
				"--health-interval", "30s",
				"--health-timeout", "5s",
				"--health-retries", "3",
				"--health-start-period", "2s",
			},
			expectTest:        []string{"CMD-SHELL", "curl -f http://localhost || exit 1"},
			expectInterval:    30 * time.Second,
			expectTimeout:     5 * time.Second,
			expectRetries:     3,
			expectStartPeriod: 2 * time.Second,
		},
		{
			name: "No_healthcheck",
			args: []string{
				"--no-healthcheck",
			},
			expectTest: []string{"NONE"},
		},
		{
			name:       "No_healthcheck_flag",
			args:       []string{},
			expectTest: nil,
		},
		{
			name: "Conflicting_flags",
			args: []string{
				"--no-healthcheck", "--health-cmd", "true",
			},
			shouldFail: true,
		},
		{
			name: "Negative_retries",
			args: []string{
				"--health-cmd", "true",
				"--health-retries", "-2",
			},
			shouldFail: true,
		},
		{
			name: "Negative_timeout",
			args: []string{
				"--health-cmd", "true",
				"--health-timeout", "-5s",
			},
			shouldFail: true,
		},
		{
			name: "Invalid_timeout_format",
			args: []string{
				"--health-cmd", "true",
				"--health-timeout", "5blah",
			},
			shouldFail: true,
		},
		{
			name: "Health_cmd_cmd_shell",
			args: []string{
				"--health-cmd", "curl -f http://localhost || exit 1",
			},
			expectTest: []string{"CMD-SHELL", "curl -f http://localhost || exit 1"},
		},
		{
			name: "Health_cmd_array_like",
			args: []string{
				"--health-cmd", "echo hello",
			},
			expectTest: []string{"CMD-SHELL", "echo hello"},
		},
		{
			name: "Health_cmd_empty",
			args: []string{
				"--health-cmd", "",
				"--health-retries", "2",
			},
			expectTest:    nil,
			expectRetries: 2,
		},
	}

	for _, tc := range testCases {
		testCase.SubTests = append(testCase.SubTests, &test.Case{
			Description: tc.name,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				args := append([]string{"run", "-d", "--name", tc.name}, tc.args...)
				args = append(args, testutil.CommonImage, "sleep", "infinity")
				return helpers.Command(args...)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				if tc.shouldFail {
					return &test.Expected{
						ExitCode: expect.ExitCodeGenericFail,
					}
				}
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: expect.All(
						func(stdout string, t tig.T) {
							inspect := nerdtest.InspectContainer(helpers, tc.name)
							hc := inspect.Config.Healthcheck
							if tc.expectTest == nil {
								assert.Assert(t, hc == nil || len(hc.Test) == 0)
							} else {
								assert.Assert(t, hc != nil)
								assert.DeepEqual(t, hc.Test, tc.expectTest)
							}
							if tc.expectRetries > 0 {
								assert.Equal(t, hc.Retries, tc.expectRetries)
							}
							if tc.expectTimeout > 0 {
								assert.Equal(t, hc.Timeout, tc.expectTimeout)
							}
							if tc.expectInterval > 0 {
								assert.Equal(t, hc.Interval, tc.expectInterval)
							}
							if tc.expectStartPeriod > 0 {
								assert.Equal(t, hc.StartPeriod, tc.expectStartPeriod)
							}
						},
					),
				}
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", tc.name)
			},
		})
	}

	testCase.Run(t)
}

func TestRunHealthcheckFromImage(t *testing.T) {
	dockerfile := fmt.Sprintf(`FROM %s
HEALTHCHECK --interval=30s --timeout=10s CMD wget -q --spider http://localhost:8080 || exit 1
	`, testutil.CommonImage)

	testCase := nerdtest.Setup()
	testCase.Require = require.All(nerdtest.Build, require.Not(nerdtest.Rootless))
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Temp().Save(dockerfile, "Dockerfile")
		data.Labels().Set("image", data.Identifier())
		helpers.Ensure("build", "-t", data.Labels().Get("image"), data.Temp().Path())
	}
	testCase.SubTests = []*test.Case{
		{
			Description: "merge_with_image",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "-d", "--name", data.Identifier(),
					"--health-retries=5",
					"--health-interval=45s",
					data.Labels().Get("image"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: expect.All(func(stdout string, t tig.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						hc := inspect.Config.Healthcheck
						assert.Assert(t, hc != nil, "expected healthcheck config to be present")
						assert.DeepEqual(t, hc.Test, []string{"CMD-SHELL", "wget -q --spider http://localhost:8080 || exit 1"})
						assert.Equal(t, 5, hc.Retries)               // From CLI flags
						assert.Equal(t, 45*time.Second, hc.Interval) // From CLI flags
						assert.Equal(t, 10*time.Second, hc.Timeout)  // From Dockerfile
					}),
				}
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
		},
		{
			Description: "Disable image health checks via runtime flag",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command(
					"run", "-d", "--name", data.Identifier(),
					"--no-healthcheck",
					data.Labels().Get("image"),
				)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: expect.All(func(stdout string, t tig.T) {
						inspect := nerdtest.InspectContainer(helpers, data.Identifier())
						hc := inspect.Config.Healthcheck
						assert.Assert(t, hc != nil, "expected healthcheck config to be present")
						assert.DeepEqual(t, hc.Test, []string{"NONE"})
					}),
				}
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
		},
	}

	testCase.Run(t)
}

func countFIFOFiles(root string) (int, error) {
	count := 0
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeNamedPipe != 0 {
			count++
		}
		return nil
	})
	return count, err
}
func TestCleanupFIFOs(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.All(
		require.Not(require.Windows),
		require.Not(nerdtest.Docker),
		require.Not(nerdtest.Rootless), // /run/containerd/fifo/ doesn't exist on rootless
	)
	testCase.NoParallel = true
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		cmd := helpers.Command("run", "-it", "--rm", testutil.CommonImage, "date")
		cmd.WithPseudoTTY()
		cmd.Run(&test.Expected{
			ExitCode: expect.ExitCodeSuccess,
		})
		oldNumFifos, err := countFIFOFiles("/run/containerd/fifo/")
		assert.NilError(t, err)

		cmd = helpers.Command("run", "-it", "--rm", testutil.CommonImage, "date")
		cmd.WithPseudoTTY()
		cmd.Run(&test.Expected{
			ExitCode: expect.ExitCodeSuccess,
		})
		newNumFifos, err := countFIFOFiles("/run/containerd/fifo/")
		assert.NilError(t, err)
		assert.Equal(t, oldNumFifos, newNumFifos)
	}
	testCase.Run(t)
}
