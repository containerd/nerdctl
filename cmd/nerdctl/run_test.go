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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
	"gotest.tools/v3/poll"
)

func TestRunEntrypointWithBuild(t *testing.T) {
	t.Parallel()
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
ENTRYPOINT ["echo", "foo"]
CMD ["echo", "bar"]
	`, testutil.CommonImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", imageName, buildCtx).AssertOK()
	base.Cmd("run", "--rm", imageName).AssertOutWithFunc(func(stdout string) error {
		expected := "foo echo bar\n"
		if stdout != expected {
			return fmt.Errorf("expected %q, got %q", expected, stdout)
		}
		return nil
	})
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
	err = os.WriteFile(path2, []byte("# this is a comment line\nTESTKEY2=TESTVAL2"), 0666)
	assert.NilError(base.T, err)

	base.Cmd("run", "--rm", "--env-file", path1, "--env-file", path2, testutil.CommonImage, "sh", "-c", "echo -n $TESTKEY1").AssertOutExactly("TESTVAL1")
	base.Cmd("run", "--rm", "--env-file", path1, "--env-file", path2, testutil.CommonImage, "sh", "-c", "echo -n $TESTKEY2").AssertOutExactly("TESTVAL2")
}

func TestRunEnv(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	base.Cmd("run", "--rm",
		"--env", "FOO=foo1,foo2",
		"--env", "BAR=bar1 bar2",
		"--env", "BAZ=",
		"--env", "QUX",
		"--env", "QUUX=quux1",
		"--env", "QUUX=quux2",
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
		return nil
	})
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
		t.Fatalf("the maximum number of old log files to retain exceeded 2 files, got: %s", matches)
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
		assert.Equal(t, 0, res.ExitCode, res.Combined())
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
		assert.Equal(t, 0, res.ExitCode, res.Combined())
		if strings.Contains(res.Stdout(), "bar") && strings.Contains(res.Stdout(), "foo") {
			found = 1
			return poll.Success()
		}
		return poll.Continue("reading from journald is not yet finished")
	}
	poll.WaitOn(t, check, poll.WithDelay(100*time.Microsecond), poll.WithTimeout(20*time.Second))
	assert.Equal(t, 1, found)
}
