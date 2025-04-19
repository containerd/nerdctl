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
	"bufio"
	"bytes"
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
	"gotest.tools/v3/poll"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
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
				func(stdout, info string, t *testing.T) {
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
	if runtime.GOOS == "windows" {
		t.Skip("json-file log driver is not yet implemented on Windows")
	}
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)

	defer base.Cmd("rm", "-f", containerName).AssertOK()
	base.Cmd("run", "-d", "--log-driver", "json-file", "--log-opt", "max-size=5K", "--log-opt", "max-file=2", "--name", containerName, testutil.CommonImage,
		"sh", "-euxc", "hexdump -C /dev/urandom | head -n1000").AssertOK()

	time.Sleep(3 * time.Second)
	inspectedContainer := base.InspectContainer(containerName)
	logJSONPath := filepath.Dir(inspectedContainer.LogPath)
	// matches = current log file + old log files to retain
	matches, err := filepath.Glob(filepath.Join(logJSONPath, inspectedContainer.ID+"*"))
	assert.NilError(t, err)
	if len(matches) != 2 {
		t.Fatalf("the number of log files is not equal to 2 files, got: %s", matches)
	}
	for _, file := range matches {
		fInfo, err := os.Stat(file)
		assert.NilError(t, err)
		// The log file size is compared to 5200 bytes (instead 5k) to keep docker compatibility.
		// Docker log rotation lacks precision because the size check is done at the log entry level
		// and not at the byte level (io.Writer), so docker log files can exceed 5k
		if fInfo.Size() > 5200 {
			t.Fatal("file size exceeded 5k")
		}
	}
}

func TestRunWithJsonFileLogDriverAndLogPathOpt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("json-file log driver is not yet implemented on Windows")
	}
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)

	defer base.Cmd("rm", "-f", containerName).AssertOK()
	customLogJSONPath := filepath.Join(t.TempDir(), containerName, containerName+"-json.log")
	base.Cmd("run", "-d", "--log-driver", "json-file", "--log-opt", fmt.Sprintf("log-path=%s", customLogJSONPath), "--log-opt", "max-size=5K", "--log-opt", "max-file=2", "--name", containerName, testutil.CommonImage,
		"sh", "-euxc", "hexdump -C /dev/urandom | head -n1000").AssertOK()

	time.Sleep(3 * time.Second)
	rawBytes, err := os.ReadFile(customLogJSONPath)
	assert.NilError(t, err)
	if len(rawBytes) == 0 {
		t.Fatalf("logs are not written correctly to log-path: %s", customLogJSONPath)
	}

	// matches = current log file + old log files to retain
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(customLogJSONPath), containerName+"*"))
	assert.NilError(t, err)
	if len(matches) != 2 {
		t.Fatalf("the number of log files is not equal to 2 files, got: %s", matches)
	}
	for _, file := range matches {
		fInfo, err := os.Stat(file)
		assert.NilError(t, err)
		if fInfo.Size() > 5200 {
			t.Fatal("file size exceeded 5k")
		}
	}
}

func TestRunWithJournaldLogDriver(t *testing.T) {
	testutil.RequireExecutable(t, "journalctl")
	journalctl, _ := exec.LookPath("journalctl")
	res := icmd.RunCmd(icmd.Command(journalctl, "-xe"))
	if res.ExitCode != 0 {
		t.Skipf("current user is not allowed to access journal logs: %s", res.Combined())
	}

	if runtime.GOOS == "windows" {
		t.Skip("journald log driver is not yet implemented on Windows")
	}
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)

	defer base.Cmd("rm", "-f", containerName).AssertOK()
	base.Cmd("run", "-d", "--log-driver", "journald", "--name", containerName, testutil.CommonImage,
		"sh", "-euxc", "echo foo; echo bar").AssertOK()

	time.Sleep(3 * time.Second)

	inspectedContainer := base.InspectContainer(containerName)

	type testCase struct {
		name   string
		filter string
	}
	testCases := []testCase{
		{
			name:   "filter journald logs using SYSLOG_IDENTIFIER field",
			filter: fmt.Sprintf("SYSLOG_IDENTIFIER=%s", inspectedContainer.ID[:12]),
		},
		{
			name:   "filter journald logs using CONTAINER_NAME field",
			filter: fmt.Sprintf("CONTAINER_NAME=%s", containerName),
		},
		{
			name:   "filter journald logs using IMAGE_NAME field",
			filter: fmt.Sprintf("IMAGE_NAME=%s", testutil.CommonImage),
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			found := 0
			check := func(log poll.LogT) poll.Result {
				res := icmd.RunCmd(icmd.Command(journalctl, "--no-pager", "--since", "2 minutes ago", tc.filter))
				assert.Equal(t, 0, res.ExitCode, res)
				if strings.Contains(res.Stdout(), "bar") && strings.Contains(res.Stdout(), "foo") {
					found = 1
					return poll.Success()
				}
				return poll.Continue("reading from journald is not yet finished")
			}
			poll.WaitOn(t, check, poll.WithDelay(100*time.Microsecond), poll.WithTimeout(20*time.Second))
			assert.Equal(t, 1, found)
		})
	}
}

func TestRunWithJournaldLogDriverAndLogOpt(t *testing.T) {
	testutil.RequireExecutable(t, "journalctl")
	journalctl, _ := exec.LookPath("journalctl")
	res := icmd.RunCmd(icmd.Command(journalctl, "-xe"))
	if res.ExitCode != 0 {
		t.Skipf("current user is not allowed to access journal logs: %s", res.Combined())
	}

	if runtime.GOOS == "windows" {
		t.Skip("journald log driver is not yet implemented on Windows")
	}
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)

	defer base.Cmd("rm", "-f", containerName).AssertOK()
	base.Cmd("run", "-d", "--log-driver", "journald", "--log-opt", "tag={{.FullID}}", "--name", containerName, testutil.CommonImage,
		"sh", "-euxc", "echo foo; echo bar").AssertOK()

	time.Sleep(3 * time.Second)
	inspectedContainer := base.InspectContainer(containerName)
	found := 0
	check := func(log poll.LogT) poll.Result {
		res := icmd.RunCmd(icmd.Command(journalctl, "--no-pager", "--since", "2 minutes ago", fmt.Sprintf("SYSLOG_IDENTIFIER=%s", inspectedContainer.ID)))
		assert.Equal(t, 0, res.ExitCode, res)
		if strings.Contains(res.Stdout(), "bar") && strings.Contains(res.Stdout(), "foo") {
			found = 1
			return poll.Success()
		}
		return poll.Continue("reading from journald is not yet finished")
	}
	poll.WaitOn(t, check, poll.WithDelay(100*time.Microsecond), poll.WithTimeout(20*time.Second))
	assert.Equal(t, 1, found)
}

func TestRunWithLogBinary(t *testing.T) {
	testutil.RequiresBuild(t)
	if runtime.GOOS == "windows" {
		t.Skip("buildkit is not enabled on windows, this feature may work on windows.")
	}
	testutil.DockerIncompatible(t)
	t.Parallel()
	base := testutil.NewBase(t)
	imageName := testutil.Identifier(t) + "-image"
	containerName := testutil.Identifier(t)

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
RUN go mod tidy
RUN go build .

FROM scratch
COPY --from=builder /go/src/logger/logger /
	`

	buildCtx := helpers.CreateBuildContext(t, dockerfile)
	tmpDir := t.TempDir()
	base.Cmd("build", buildCtx, "--output", fmt.Sprintf("type=local,src=/go/src/logger/logger,dest=%s", tmpDir)).AssertOK()
	defer base.Cmd("image", "rm", "-f", imageName).AssertOK()

	base.Cmd("container", "rm", "-f", containerName).AssertOK()
	base.Cmd("run", "-d", "--log-driver", fmt.Sprintf("binary://%s/logger", tmpDir), "--name", containerName, testutil.CommonImage,
		"sh", "-euxc", "echo foo; echo bar").AssertOK()
	defer base.Cmd("container", "rm", "-f", containerName).AssertOK()

	inspectedContainer := base.InspectContainer(containerName)
	bytes, err := os.ReadFile(filepath.Join(os.TempDir(), fmt.Sprintf("%s_%s.log", inspectedContainer.ID, "stdout")))
	assert.NilError(t, err)
	log := string(bytes)
	assert.Check(t, strings.Contains(log, "foo"))
	assert.Check(t, strings.Contains(log, "bar"))
}

// history: There was a bug that the --add-host items disappear when the another container created.
// This test ensures that it doesn't happen.
// (https://github.com/containerd/nerdctl/issues/2560)
func TestRunAddHostRemainsWhenAnotherContainerCreated(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ocihook is not yet supported on Windows")
	}
	base := testutil.NewBase(t)

	containerName := testutil.Identifier(t)
	hostMapping := "test-add-host:10.0.0.1"
	base.Cmd("run", "-d", "--add-host", hostMapping, "--name", containerName, testutil.CommonImage, "sleep", nerdtest.Infinity).AssertOK()
	defer base.Cmd("container", "rm", "-f", containerName).Run()

	checkEtcHosts := func(stdout string) error {
		matcher, err := regexp.Compile(`^10.0.0.1\s+test-add-host$`)
		if err != nil {
			return err
		}
		var found bool
		sc := bufio.NewScanner(bytes.NewBufferString(stdout))
		for sc.Scan() {
			if matcher.Match(sc.Bytes()) {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("host not found")
		}
		return nil
	}
	base.Cmd("exec", containerName, "cat", "/etc/hosts").AssertOutWithFunc(checkEtcHosts)

	// run another container
	base.Cmd("run", "--rm", testutil.CommonImage).AssertOK()

	base.Cmd("exec", containerName, "cat", "/etc/hosts").AssertOutWithFunc(checkEtcHosts)
}

// https://github.com/containerd/nerdctl/issues/2726
func TestRunRmTime(t *testing.T) {
	base := testutil.NewBase(t)
	base.Cmd("pull", "--quiet", testutil.CommonImage)
	t0 := time.Now()
	base.Cmd("run", "--rm", testutil.CommonImage, "true").AssertOK()
	t1 := time.Now()
	took := t1.Sub(t0)
	const deadline = 3 * time.Second
	if took > deadline {
		t.Fatalf("expected to have completed in %v, took %v", deadline, took)
	}
}

func runAttachStdin(t *testing.T, testStr string, args []string) string {
	if runtime.GOOS == "windows" {
		t.Skip("run attach test is not yet implemented on Windows")
	}

	t.Parallel()
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)

	opts := []func(*testutil.Cmd){
		testutil.WithStdin(strings.NewReader("echo " + testStr + "\nexit\n")),
	}

	fullArgs := []string{"run", "--rm", "-i"}
	fullArgs = append(fullArgs, args...)
	fullArgs = append(fullArgs,
		"--name",
		containerName,
		testutil.CommonImage,
	)

	defer base.Cmd("rm", "-f", containerName).AssertOK()
	result := base.Cmd(fullArgs...).CmdOption(opts...).Run()

	return result.Combined()
}

func runAttach(t *testing.T, testStr string, args []string) string {
	if runtime.GOOS == "windows" {
		t.Skip("run attach test is not yet implemented on Windows")
	}

	t.Parallel()
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)

	fullArgs := []string{"run"}
	fullArgs = append(fullArgs, args...)
	fullArgs = append(fullArgs,
		"--name",
		containerName,
		testutil.CommonImage,
		"sh",
		"-euxc",
		"echo "+testStr,
	)

	defer base.Cmd("rm", "-f", containerName).AssertOK()
	result := base.Cmd(fullArgs...).Run()

	return result.Combined()
}

func TestRunAttachFlag(t *testing.T) {

	type testCase struct {
		name        string
		args        []string
		testFunc    func(t *testing.T, testStr string, args []string) string
		testStr     string
		expectedOut string
		dockerOut   string
	}
	testCases := []testCase{
		{
			name:        "AttachFlagStdin",
			args:        []string{"-a", "STDIN", "-a", "STDOUT"},
			testFunc:    runAttachStdin,
			testStr:     "test-run-stdio",
			expectedOut: "test-run-stdio",
			dockerOut:   "test-run-stdio",
		},
		{
			name:        "AttachFlagStdOut",
			args:        []string{"-a", "STDOUT"},
			testFunc:    runAttach,
			testStr:     "foo",
			expectedOut: "foo",
			dockerOut:   "foo",
		},
		{
			name:        "AttachFlagMixedValue",
			args:        []string{"-a", "STDIN", "-a", "invalid-value"},
			testFunc:    runAttach,
			testStr:     "foo",
			expectedOut: "invalid stream specified with -a flag. Valid streams are STDIN, STDOUT, and STDERR",
			dockerOut:   "valid streams are STDIN, STDOUT and STDERR",
		},
		{
			name:        "AttachFlagInvalidValue",
			args:        []string{"-a", "invalid-stream"},
			testFunc:    runAttach,
			testStr:     "foo",
			expectedOut: "invalid stream specified with -a flag. Valid streams are STDIN, STDOUT, and STDERR",
			dockerOut:   "valid streams are STDIN, STDOUT and STDERR",
		},
		{
			name:        "AttachFlagCaseInsensitive",
			args:        []string{"-a", "stdin", "-a", "stdout"},
			testFunc:    runAttachStdin,
			testStr:     "test-run-stdio",
			expectedOut: "test-run-stdio",
			dockerOut:   "test-run-stdio",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			actualOut := tc.testFunc(t, tc.testStr, tc.args)
			errorMsg := fmt.Sprintf("%s failed;\nExpected: '%s'\nActual: '%s'", tc.name, tc.expectedOut, actualOut)
			if testutil.GetTarget() == testutil.Docker {
				assert.Equal(t, true, strings.Contains(actualOut, tc.dockerOut), errorMsg)
			} else {
				assert.Equal(t, true, strings.Contains(actualOut, tc.expectedOut), errorMsg)
			}
		})
	}
}

func TestRunQuiet(t *testing.T) {
	base := testutil.NewBase(t)

	teardown := func() {
		base.Cmd("rmi", "-f", testutil.CommonImage).Run()
	}
	defer teardown()
	teardown()

	sentinel := "test run quiet"
	result := base.Cmd("run", "--rm", "--quiet", testutil.CommonImage, fmt.Sprintf(`echo "%s"`, sentinel)).Run()
	assert.Assert(t, strings.Contains(result.Combined(), sentinel))

	wasQuiet := func(output, sentinel string) bool {
		return !strings.Contains(output, sentinel)
	}

	// Docker and nerdctl image pulls are not 1:1.
	if testutil.GetTarget() == testutil.Docker {
		sentinel = "Pull complete"
	} else {
		sentinel = "resolved"
	}

	assert.Assert(t, wasQuiet(result.Combined(), sentinel), "Found %s in container run output", sentinel)
}

func TestRunFromOCIArchive(t *testing.T) {
	testutil.RequiresBuild(t)
	testutil.RegisterBuildCacheCleanup(t)

	// Docker does not support running container images from OCI archive.
	testutil.DockerIncompatible(t)

	base := testutil.NewBase(t)
	imageName := testutil.Identifier(t)

	teardown := func() {
		base.Cmd("rmi", "-f", imageName).Run()
	}
	defer teardown()
	teardown()

	const sentinel = "test-nerdctl-run-from-oci-archive"
	dockerfile := fmt.Sprintf(`FROM %s
	CMD ["echo", "%s"]`, testutil.CommonImage, sentinel)

	buildCtx := helpers.CreateBuildContext(t, dockerfile)
	tag := fmt.Sprintf("%s:latest", imageName)
	tarPath := fmt.Sprintf("%s/%s.tar", buildCtx, imageName)

	base.Cmd("build", "--tag", tag, fmt.Sprintf("--output=type=oci,dest=%s", tarPath), buildCtx).AssertOK()
	base.Cmd("run", "--rm", fmt.Sprintf("oci-archive://%s", tarPath)).AssertOutContainsAll(fmt.Sprintf("Loaded image: %s", tag), sentinel)
}

func TestRunDomainname(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("run --hostname not implemented on Windows yet")
	}

	testCases := []struct {
		name        string
		hostname    string
		domainname  string
		Cmd         string
		CmdFlag     string
		expectedOut string
	}{
		{
			name:        "Check domain name",
			hostname:    "foobar",
			domainname:  "example.com",
			Cmd:         "hostname",
			CmdFlag:     "-d",
			expectedOut: "example.com",
		},
		{
			name:        "check fqdn",
			hostname:    "foobar",
			domainname:  "example.com",
			Cmd:         "hostname",
			CmdFlag:     "-f",
			expectedOut: "foobar.example.com",
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			base := testutil.NewBase(t)

			base.Cmd("run",
				"--rm",
				"--hostname", tc.hostname,
				"--domainname", tc.domainname,
				testutil.CommonImage,
				tc.Cmd,
				tc.CmdFlag,
			).AssertOutContains(tc.expectedOut)
		})
	}
}
