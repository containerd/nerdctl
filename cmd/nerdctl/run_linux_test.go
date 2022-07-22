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
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/containerd/nerdctl/pkg/testutil"

	"gotest.tools/v3/assert"
)

func TestRunCustomRootfs(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	rootfs := prepareCustomRootfs(base, testutil.AlpineImage)
	defer os.RemoveAll(rootfs)
	base.Cmd("run", "--rm", "--rootfs", rootfs, "/bin/cat", "/proc/self/environ").AssertOutContains("PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	base.Cmd("run", "--rm", "--entrypoint", "/bin/echo", "--rootfs", rootfs, "echo", "foo").AssertOutExactly("echo foo\n")
}

func prepareCustomRootfs(base *testutil.Base, imageName string) string {
	base.Cmd("pull", imageName).AssertOK()
	tmpDir, err := os.MkdirTemp(base.T.TempDir(), "test-save")
	assert.NilError(base.T, err)
	defer os.RemoveAll(tmpDir)
	archiveTarPath := filepath.Join(tmpDir, "a.tar")
	base.Cmd("save", "-o", archiveTarPath, imageName).AssertOK()
	rootfs, err := os.MkdirTemp(base.T.TempDir(), "rootfs")
	assert.NilError(base.T, err)
	err = extractDockerArchive(archiveTarPath, rootfs)
	assert.NilError(base.T, err)
	return rootfs
}

func TestRunShmSize(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	const shmSize = "32m"

	base.Cmd("run", "--rm", "--shm-size", shmSize, testutil.AlpineImage, "/bin/grep", "shm", "/proc/self/mounts").AssertOutContains("size=32768k")
}

func TestRunPidHost(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	pid := os.Getpid()

	base.Cmd("run", "--rm", "--pid=host", testutil.AlpineImage, "ps", "auxw").AssertOutContains(strconv.Itoa(pid))
}

func TestRunIpcHost(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	testFilePath := filepath.Join("/dev/shm",
		fmt.Sprintf("%s-%d-%s", testutil.Identifier(t), os.Geteuid(), base.Target))
	err := os.WriteFile(testFilePath, []byte(""), 0644)
	assert.NilError(base.T, err)
	defer os.Remove(testFilePath)

	base.Cmd("run", "--rm", "--ipc=host", testutil.AlpineImage, "ls", testFilePath).AssertOK()
}

func TestRunAddHost(t *testing.T) {
	// Not parallelizable (https://github.com/containerd/nerdctl/issues/1127)
	base := testutil.NewBase(t)
	base.Cmd("run", "--rm", "--add-host", "testing.example.com:10.0.0.1", testutil.AlpineImage, "cat", "/etc/hosts").AssertOutWithFunc(func(stdout string) error {
		var found bool
		sc := bufio.NewScanner(bytes.NewBufferString(stdout))
		for sc.Scan() {
			//removing spaces and tabs separating items
			line := strings.ReplaceAll(sc.Text(), " ", "")
			line = strings.ReplaceAll(line, "\t", "")
			if strings.Contains(line, "10.0.0.1testing.example.com") {
				found = true
			}
		}
		if !found {
			return errors.New("host was not added")
		}
		return nil
	})
	base.Cmd("run", "--rm", "--add-host", "test:10.0.0.1", "--add-host", "test1:10.0.0.1", testutil.AlpineImage, "cat", "/etc/hosts").AssertOutWithFunc(func(stdout string) error {
		var found int
		sc := bufio.NewScanner(bytes.NewBufferString(stdout))
		for sc.Scan() {
			//removing spaces and tabs separating items
			line := strings.ReplaceAll(sc.Text(), " ", "")
			line = strings.ReplaceAll(line, "\t", "")
			if strutil.InStringSlice([]string{"10.0.0.1test", "10.0.0.1test1"}, line) {
				found += 1
			}
		}
		if found != 2 {
			return fmt.Errorf("host was not added, found %d", found)
		}
		return nil
	})
	base.Cmd("run", "--rm", "--add-host", "10.0.0.1:testing.example.com", testutil.AlpineImage, "cat", "/etc/hosts").AssertFail()
}

func TestRunUlimit(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	ulimit := "nofile=622:622"
	ulimit2 := "nofile=622:722"

	base.Cmd("run", "--rm", "--ulimit", ulimit, testutil.AlpineImage, "sh", "-c", "ulimit -Sn").AssertOutExactly("622\n")
	base.Cmd("run", "--rm", "--ulimit", ulimit, testutil.AlpineImage, "sh", "-c", "ulimit -Hn").AssertOutExactly("622\n")

	base.Cmd("run", "--rm", "--ulimit", ulimit2, testutil.AlpineImage, "sh", "-c", "ulimit -Sn").AssertOutExactly("622\n")
	base.Cmd("run", "--rm", "--ulimit", ulimit2, testutil.AlpineImage, "sh", "-c", "ulimit -Hn").AssertOutExactly("722\n")
}

func TestRunWithInit(t *testing.T) {
	t.Parallel()
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)

	container := testutil.Identifier(t)
	base.Cmd("run", "-d", "--name", container, testutil.AlpineImage, "sleep", "infinity").AssertOK()
	defer base.Cmd("rm", "-f", container).Run()

	base.Cmd("stop", "--time=3", container).AssertOK()
	// Unable to handle TERM signal, be killed when timeout
	assert.Equal(t, base.InspectContainer(container).State.ExitCode, 137)

	// Test with --init-path
	container1 := container + "-1"
	base.Cmd("run", "-d", "--name", container1, "--init-binary", "tini-custom",
		testutil.AlpineImage, "sleep", "infinity").AssertOK()
	defer base.Cmd("rm", "-f", container1).Run()

	base.Cmd("stop", "--time=3", container1).AssertOK()
	assert.Equal(t, base.InspectContainer(container1).State.ExitCode, 143)

	// Test with --init
	container2 := container + "-2"
	base.Cmd("run", "-d", "--name", container2, "--init",
		testutil.AlpineImage, "sleep", "infinity").AssertOK()
	defer base.Cmd("rm", "-f", container2).Run()

	base.Cmd("stop", "--time=3", container2).AssertOK()
	assert.Equal(t, base.InspectContainer(container2).State.ExitCode, 143)
}

func TestRunTTY(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	if testutil.GetTarget() == testutil.Nerdctl {
		testutil.RequireDaemonVersion(base, ">= 1.6.0-0")
	}

	const sttyPartialOutput = "speed 38400 baud"
	// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
	// unbuffer(1) can be installed with `apt-get install expect`.
	unbuffer := []string{"unbuffer"}
	base.CmdWithHelper(unbuffer, "run", "--rm", "-it", testutil.CommonImage, "stty").AssertOutContains(sttyPartialOutput)
	base.CmdWithHelper(unbuffer, "run", "--rm", "-t", testutil.CommonImage, "stty").AssertOutContains(sttyPartialOutput)
	base.Cmd("run", "--rm", "-i", testutil.CommonImage, "stty").AssertFail()
	base.Cmd("run", "--rm", testutil.CommonImage, "stty").AssertFail()
}

func TestRunWithFluentdLogDriver(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fluentd log driver is not yet implemented on Windows")
	}
	base := testutil.NewBase(t)
	tempDirectory := t.TempDir()
	err := os.Chmod(tempDirectory, 0777)
	assert.NilError(t, err)

	containerName := testutil.Identifier(t)
	base.Cmd("run", "-d", "--name", containerName, "-p", "24224:24224",
		"-v", fmt.Sprintf("%s:/fluentd/log", tempDirectory), testutil.FluentdImage).AssertOK()
	defer base.Cmd("rm", "-f", containerName).AssertOK()
	time.Sleep(3 * time.Second)

	testContainerName := containerName + "test"
	base.Cmd("run", "-d", "--log-driver", "fluentd", "--name", testContainerName, testutil.CommonImage,
		"sh", "-c", "echo test").AssertOK()
	defer base.Cmd("rm", "-f", testContainerName).AssertOK()

	inspectedContainer := base.InspectContainer(testContainerName)
	matches, err := filepath.Glob(tempDirectory + "/" + "data.*.log")
	assert.NilError(t, err)
	assert.Equal(t, 1, len(matches))

	data, err := os.ReadFile(matches[0])
	assert.NilError(t, err)
	logData := string(data)
	assert.Equal(t, true, strings.Contains(logData, "test"))
	assert.Equal(t, true, strings.Contains(logData, inspectedContainer.ID))
}

func TestRunWithFluentdLogDriverWithLogOpt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fluentd log driver is not yet implemented on Windows")
	}
	base := testutil.NewBase(t)
	tempDirectory := t.TempDir()
	err := os.Chmod(tempDirectory, 0777)
	assert.NilError(t, err)

	containerName := testutil.Identifier(t)
	base.Cmd("run", "-d", "--name", containerName, "-p", "24225:24224",
		"-v", fmt.Sprintf("%s:/fluentd/log", tempDirectory), testutil.FluentdImage).AssertOK()
	defer base.Cmd("rm", "-f", containerName).AssertOK()
	time.Sleep(3 * time.Second)

	testContainerName := containerName + "test"
	base.Cmd("run", "-d", "--log-driver", "fluentd", "--log-opt", "fluentd-address=127.0.0.1:24225",
		"--name", testContainerName, testutil.CommonImage, "sh", "-c", "echo test2").AssertOK()
	defer base.Cmd("rm", "-f", testContainerName).AssertOK()

	inspectedContainer := base.InspectContainer(testContainerName)
	matches, err := filepath.Glob(tempDirectory + "/" + "data.*.log")
	assert.NilError(t, err)
	assert.Equal(t, 1, len(matches))

	data, err := os.ReadFile(matches[0])
	assert.NilError(t, err)
	logData := string(data)
	assert.Equal(t, true, strings.Contains(logData, "test2"))
	assert.Equal(t, true, strings.Contains(logData, inspectedContainer.ID))
}
