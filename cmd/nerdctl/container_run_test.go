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

package main

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

	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestRunEntrypointWithBuild(t *testing.T) {
	t.Parallel()
	testutil.RequiresBuild(t)
	testutil.RegisterBuildCacheCleanup(t)
	base := testutil.NewBase(t)
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
ENTRYPOINT ["echo", "foo"]
CMD ["echo", "bar"]
	`, testutil.CommonImage)

	buildCtx := createBuildContext(t, dockerfile)

	base.Cmd("build", "-t", imageName, buildCtx).AssertOK()
	base.Cmd("run", "--rm", imageName).AssertOutExactly("foo echo bar\n")
	base.Cmd("run", "--rm", "--entrypoint", "", imageName).AssertFail()
	base.Cmd("run", "--rm", "--entrypoint", "", imageName, "echo", "blah").AssertOutWithFunc(func(stdout string) error {
		if !strings.Contains(stdout, "blah") {
			return errors.New("echo blah was not executed?")
		}
		if strings.Contains(stdout, "bar") {
			return errors.New("echo bar should not be executed")
		}
		if strings.Contains(stdout, "foo") {
			return errors.New("echo foo should not be executed")
		}
		return nil
	})
	base.Cmd("run", "--rm", "--entrypoint", "time", imageName).AssertFail()
	base.Cmd("run", "--rm", "--entrypoint", "time", imageName, "echo", "blah").AssertOutWithFunc(func(stdout string) error {
		if !strings.Contains(stdout, "blah") {
			return errors.New("echo blah was not executed?")
		}
		if strings.Contains(stdout, "bar") {
			return errors.New("echo bar should not be executed")
		}
		if strings.Contains(stdout, "foo") {
			return errors.New("echo foo should not be executed")
		}
		return nil
	})
}

func TestRunWorkdir(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	dir := "/foo"
	if runtime.GOOS == "windows" {
		dir = "c:" + dir
	}
	cmd := base.Cmd("run", "--rm", "--workdir="+dir, testutil.CommonImage, "pwd")
	cmd.AssertOutContains("/foo")
}

func TestRunWithDoubleDash(t *testing.T) {
	t.Parallel()
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	base.Cmd("run", "--rm", testutil.CommonImage, "--", "sh", "-euxc", "exit 0").AssertOK()
}

func TestRunExitCode(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)
	testContainer0 := tID + "-0"
	testContainer123 := tID + "-123"
	defer base.Cmd("rm", "-f", testContainer0, testContainer123).Run()

	base.Cmd("run", "--name", testContainer0, testutil.CommonImage, "sh", "-euxc", "exit 0").AssertOK()
	base.Cmd("run", "--name", testContainer123, testutil.CommonImage, "sh", "-euxc", "exit 123").AssertExitCode(123)
	base.Cmd("ps", "-a").AssertOutWithFunc(func(stdout string) error {
		if !strings.Contains(stdout, "Exited (0)") {
			return fmt.Errorf("no entry for %q", testContainer0)
		}
		if !strings.Contains(stdout, "Exited (123)") {
			return fmt.Errorf("no entry for %q", testContainer123)
		}
		return nil
	})

	inspect0 := base.InspectContainer(testContainer0)
	assert.Equal(base.T, "exited", inspect0.State.Status)
	assert.Equal(base.T, 0, inspect0.State.ExitCode)

	inspect123 := base.InspectContainer(testContainer123)
	assert.Equal(base.T, "exited", inspect123.State.Status)
	assert.Equal(base.T, 123, inspect123.State.ExitCode)
}

func TestRunCIDFile(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	fileName := filepath.Join(t.TempDir(), "cid.file")

	base.Cmd("run", "--rm", "--cidfile", fileName, testutil.CommonImage).AssertOK()
	defer os.Remove(fileName)

	_, err := os.Stat(fileName)
	assert.NilError(base.T, err)

	base.Cmd("run", "--rm", "--cidfile", fileName, testutil.CommonImage).AssertFail()
}

func TestRunEnvFile(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	base.Env = append(base.Env, "HOST_ENV=ENV-IN-HOST")

	tID := testutil.Identifier(t)
	file1, err := os.CreateTemp("", tID)
	assert.NilError(base.T, err)
	path1 := file1.Name()
	defer file1.Close()
	defer os.Remove(path1)
	err = os.WriteFile(path1, []byte("# this is a comment line\nTESTKEY1=TESTVAL1"), 0666)
	assert.NilError(base.T, err)

	file2, err := os.CreateTemp("", tID)
	assert.NilError(base.T, err)
	path2 := file2.Name()
	defer file2.Close()
	defer os.Remove(path2)
	err = os.WriteFile(path2, []byte("# this is a comment line\nTESTKEY2=TESTVAL2\nHOST_ENV"), 0666)
	assert.NilError(base.T, err)

	base.Cmd("run", "--rm", "--env-file", path1, "--env-file", path2, testutil.CommonImage, "sh", "-c", "echo -n $TESTKEY1").AssertOutExactly("TESTVAL1")
	base.Cmd("run", "--rm", "--env-file", path1, "--env-file", path2, testutil.CommonImage, "sh", "-c", "echo -n $TESTKEY2").AssertOutExactly("TESTVAL2")
	base.Cmd("run", "--rm", "--env-file", path1, "--env-file", path2, testutil.CommonImage, "sh", "-c", "echo -n $HOST_ENV").AssertOutExactly("ENV-IN-HOST")
}

func TestRunEnv(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	base.Env = append(base.Env, "CORGE=corge-value-in-host", "GARPLY=garply-value-in-host")
	base.Cmd("run", "--rm",
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

		testutil.CommonImage, "env").AssertOutWithFunc(func(stdout string) error {
		if !strings.Contains(stdout, "\nFOO=foo1,foo2\n") {
			return errors.New("got bad FOO")
		}
		if !strings.Contains(stdout, "\nBAR=bar1 bar2\n") {
			return errors.New("got bad BAR")
		}
		if !strings.Contains(stdout, "\nBAZ=\n") && runtime.GOOS != "windows" {
			return errors.New("got bad BAZ")
		}
		if strings.Contains(stdout, "QUX") {
			return errors.New("got bad QUX (should not be set)")
		}
		if !strings.Contains(stdout, "\nQUUX=quux2\n") {
			return errors.New("got bad QUUX")
		}
		if !strings.Contains(stdout, "\nCORGE=corge-value-in-host\n") {
			return errors.New("got bad CORGE")
		}
		if !strings.Contains(stdout, "\nGRAULT=grault_key=grault_value\n") {
			return errors.New("got bad GRAULT")
		}
		if !strings.Contains(stdout, "\nGARPLY=\n") && runtime.GOOS != "windows" {
			return errors.New("got bad GARPLY")
		}
		if !strings.Contains(stdout, "\nWALDO=\n") && runtime.GOOS != "windows" {
			return errors.New("got bad WALDO")
		}

		return nil
	})
}
func TestRunHostnameEnv(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)

	base.Cmd("run", "-i", "--rm", testutil.CommonImage).
		CmdOption(testutil.WithStdin(strings.NewReader(`[[ "HOSTNAME=$(hostname)" == "$(env | grep HOSTNAME)" ]]`))).
		AssertOK()

	if runtime.GOOS == "windows" {
		t.Skip("run --hostname not implemented on Windows yet")
	}
	base.Cmd("run", "--rm", "--hostname", "foobar", testutil.CommonImage, "env").AssertOutContains("HOSTNAME=foobar")
}

func TestRunStdin(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	if testutil.GetTarget() == testutil.Nerdctl {
		testutil.RequireDaemonVersion(base, ">= 1.6.0-0")
	}

	const testStr = "test-run-stdin"
	opts := []func(*testutil.Cmd){
		testutil.WithStdin(strings.NewReader(testStr)),
	}
	base.Cmd("run", "--rm", "-i", testutil.CommonImage, "cat").CmdOption(opts...).AssertOutExactly(testStr)
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
	if runtime.GOOS == "windows" {
		t.Skip("journald log driver is not yet implemented on Windows")
	}
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)

	defer base.Cmd("rm", "-f", containerName).AssertOK()
	base.Cmd("run", "-d", "--log-driver", "journald", "--name", containerName, testutil.CommonImage,
		"sh", "-euxc", "echo foo; echo bar").AssertOK()

	time.Sleep(3 * time.Second)
	journalctl, err := exec.LookPath("journalctl")
	assert.NilError(t, err)
	inspectedContainer := base.InspectContainer(containerName)
	found := 0
	check := func(log poll.LogT) poll.Result {
		res := icmd.RunCmd(icmd.Command(journalctl, "--no-pager", "--since", "2 minutes ago", fmt.Sprintf("SYSLOG_IDENTIFIER=%s", inspectedContainer.ID[:12])))
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

func TestRunWithJournaldLogDriverAndLogOpt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("journald log driver is not yet implemented on Windows")
	}
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)

	defer base.Cmd("rm", "-f", containerName).AssertOK()
	base.Cmd("run", "-d", "--log-driver", "journald", "--log-opt", "tag={{.FullID}}", "--name", containerName, testutil.CommonImage,
		"sh", "-euxc", "echo foo; echo bar").AssertOK()

	time.Sleep(3 * time.Second)
	journalctl, err := exec.LookPath("journalctl")
	assert.NilError(t, err)
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

	const dockerfile = `
FROM golang:latest as builder
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

	buildCtx := createBuildContext(t, dockerfile)
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

func TestRunWithTtyAndDetached(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("json-file log driver is not yet implemented on Windows")
	}
	base := testutil.NewBase(t)
	imageName := testutil.CommonImage
	withoutTtyContainerName := "without-terminal-" + testutil.Identifier(t)
	withTtyContainerName := "with-terminal-" + testutil.Identifier(t)

	// without -t, fail
	base.Cmd("run", "-d", "--name", withoutTtyContainerName, imageName, "stty").AssertOK()
	defer base.Cmd("container", "rm", "-f", withoutTtyContainerName).AssertOK()
	base.Cmd("logs", withoutTtyContainerName).AssertCombinedOutContains("stty: standard input: Not a tty")
	withoutTtyContainer := base.InspectContainer(withoutTtyContainerName)
	assert.Equal(base.T, 1, withoutTtyContainer.State.ExitCode)

	// with -t, success
	base.Cmd("run", "-d", "-t", "--name", withTtyContainerName, imageName, "stty").AssertOK()
	defer base.Cmd("container", "rm", "-f", withTtyContainerName).AssertOK()
	base.Cmd("logs", withTtyContainerName).AssertCombinedOutContains("speed 38400 baud; line = 0;")
	withTtyContainer := base.InspectContainer(withTtyContainerName)
	assert.Equal(base.T, 0, withTtyContainer.State.ExitCode)
}

// history: There was a bug that the --add-host items disappear when the another container created.
// This case ensures that it's doesn't happen.
// (https://github.com/containerd/nerdctl/issues/2560)
func TestRunAddHostRemainsWhenAnotherContainerCreated(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ocihook is not yet supported on Windows")
	}
	base := testutil.NewBase(t)

	containerName := testutil.Identifier(t)
	hostMapping := "test-add-host:10.0.0.1"
	base.Cmd("run", "-d", "--add-host", hostMapping, "--name", containerName, testutil.CommonImage, "sleep", "infinity").AssertOK()
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
	base.Cmd("pull", testutil.CommonImage)
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
