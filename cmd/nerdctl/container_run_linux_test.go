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
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

func TestRunCustomRootfs(t *testing.T) {
	testutil.DockerIncompatible(t)
	// FIXME: root issue is undiagnosed and this is very likely a containerd bug
	// It appears that in certain conditions, the proxy content store info method will fail on the layer of the image
	// Search for func (pcs *proxyContentStore) ReaderAt(ctx context.Context, desc ocispec.Descriptor) (content.ReaderAt, error) {
	// Note that:
	// - the problem is still here with containerd and nerdctl v2
	// - it seems to affect images that are tagged multiple times, or that share a layer with another image
	// - this test is not parallelized - but the fact that namespacing it solves the problem suggest that something
	// happening in the default namespace BEFORE this test is run is SOMETIMES setting conditions that will make this fail
	// Possible suspects would be concurrent pulls somehow effing things up w. namespaces.
	base := testutil.NewBaseWithNamespace(t, testutil.Identifier(t))
	rootfs := prepareCustomRootfs(base, testutil.AlpineImage)
	t.Cleanup(func() {
		base.Cmd("namespace", "remove", testutil.Identifier(t)).Run()
	})
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

func TestRunShmSizeIPCShareable(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	const shmSize = "32m"

	container := testutil.Identifier(t)
	base.Cmd("run", "--rm", "--name", container, "--ipc", "shareable", "--shm-size", shmSize, testutil.AlpineImage, "/bin/grep", "shm", "/proc/self/mounts").AssertOutContains("size=32768k")
	defer base.Cmd("rm", "-f", container)
}

func TestRunIPCShareableRemoveMount(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	container := testutil.Identifier(t)

	base.Cmd("run", "--name", container, "--ipc", "shareable", testutil.AlpineImage, "sleep", "0").AssertOK()
	base.Cmd("rm", container).AssertOK()
}

func TestRunIPCContainerNotExists(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)

	container := testutil.Identifier(t)
	result := base.Cmd("run", "--name", container, "--ipc", "container:abcd1234", testutil.AlpineImage, "sleep", "infinity").Run()
	defer base.Cmd("rm", "-f", container)
	combined := result.Combined()
	if !strings.Contains(strings.ToLower(combined), "no such container: abcd1234") {
		t.Fatalf("unexpected output: %s", combined)
	}
}

func TestRunShmSizeIPCContainer(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)

	const shmSize = "32m"
	sharedContainerResult := base.Cmd("run", "-d", "--ipc", "shareable", "--shm-size", shmSize, testutil.AlpineImage, "sleep", "infinity").Run()
	baseContainerID := strings.TrimSpace(sharedContainerResult.Stdout())
	defer base.Cmd("rm", "-f", baseContainerID).Run()

	base.Cmd("run", "--rm", fmt.Sprintf("--ipc=container:%s", baseContainerID),
		testutil.AlpineImage, "/bin/grep", "shm", "/proc/self/mounts").AssertOutContains("size=32768k")
}

func TestRunIPCContainer(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)

	const shmSize = "32m"
	victimContainerResult := base.Cmd("run", "-d", "--ipc", "shareable", "--shm-size", shmSize, testutil.AlpineImage, "sleep", "infinity").Run()
	victimContainerID := strings.TrimSpace(victimContainerResult.Stdout())
	defer base.Cmd("rm", "-f", victimContainerID).Run()

	base.Cmd("run", "--rm", fmt.Sprintf("--ipc=container:%s", victimContainerID),
		testutil.AlpineImage, "/bin/grep", "shm", "/proc/self/mounts").AssertOutContains("size=32768k")
}

func TestRunPidHost(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	pid := os.Getpid()

	base.Cmd("run", "--rm", "--pid=host", testutil.AlpineImage, "ps", "auxw").AssertOutContains(strconv.Itoa(pid))
}

func TestRunUtsHost(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)

	// Was thinking of os.ReadLink("/proc/1/ns/uts")
	// but you'd get EPERM for rootless. Just validate the
	// hostname is the same.
	hostName, err := os.Hostname()
	assert.NilError(base.T, err)

	base.Cmd("run", "--rm", "--uts=host", testutil.AlpineImage, "hostname").AssertOutContains(hostName)
	// Validate we can't provide a hostname with uts=host
	base.Cmd("run", "--rm", "--uts=host", "--hostname=foobar", testutil.AlpineImage, "hostname").AssertFail()
}

func TestRunPidContainer(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)

	sharedContainerResult := base.Cmd("run", "-d", testutil.AlpineImage, "sleep", "infinity").Run()
	baseContainerID := strings.TrimSpace(sharedContainerResult.Stdout())
	defer base.Cmd("rm", "-f", baseContainerID).Run()

	base.Cmd("run", "--rm", fmt.Sprintf("--pid=container:%s", baseContainerID),
		testutil.AlpineImage, "ps", "ax").AssertOutContains("sleep infinity")
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
				found++
			}
		}
		if found != 2 {
			return fmt.Errorf("host was not added, found %d", found)
		}
		return nil
	})
	base.Cmd("run", "--rm", "--add-host", "10.0.0.1:testing.example.com", testutil.AlpineImage, "cat", "/etc/hosts").AssertFail()

	response := "This is the expected response for --add-host special IP test."
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, response)
	})
	const hostPort = 8081
	s := http.Server{Addr: fmt.Sprintf(":%d", hostPort), Handler: nil, ReadTimeout: 30 * time.Second}
	go s.ListenAndServe()
	defer s.Shutdown(context.Background())
	base.Cmd("run", "--rm", "--add-host", "test:host-gateway", testutil.NginxAlpineImage, "curl", fmt.Sprintf("test:%d", hostPort)).AssertOutExactly(response)
}

func TestRunAddHostWithCustomHostGatewayIP(t *testing.T) {
	// Not parallelizable (https://github.com/containerd/nerdctl/issues/1127)
	base := testutil.NewBase(t)
	testutil.DockerIncompatible(t)
	base.Cmd("run", "--rm", "--host-gateway-ip", "192.168.5.2", "--add-host", "test:host-gateway", testutil.AlpineImage, "cat", "/etc/hosts").AssertOutWithFunc(func(stdout string) error {
		var found bool
		sc := bufio.NewScanner(bytes.NewBufferString(stdout))
		for sc.Scan() {
			//removing spaces and tabs separating items
			line := strings.ReplaceAll(sc.Text(), " ", "")
			line = strings.ReplaceAll(line, "\t", "")
			if strings.Contains(line, "192.168.5.2test") {
				found = true
			}
		}
		if !found {
			return errors.New("host was not added")
		}
		return nil
	})
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

	// tests pipe works
	res := icmd.RunCmd(icmd.Command("unbuffer", "/bin/sh", "-c", fmt.Sprintf("%q run --rm -it %q echo hi | grep hi", base.Binary, testutil.CommonImage)))
	assert.Equal(t, 0, res.ExitCode, res.Combined())
}

func runSigProxy(t *testing.T, args ...string) (string, bool, bool) {
	t.Parallel()
	base := testutil.NewBase(t)
	testContainerName := testutil.Identifier(t)
	defer base.Cmd("rm", "-f", testContainerName).Run()

	fullArgs := []string{"run"}
	fullArgs = append(fullArgs, args...)
	fullArgs = append(fullArgs,
		"--name",
		testContainerName,
		testutil.CommonImage,
		"sh",
		"-c",
		testutil.SigProxyTestScript,
	)

	result := base.Cmd(fullArgs...).Start()
	process := result.Cmd.Process

	// Waits until we reach the trap command in the shell script, then sends SIGINT.
	time.Sleep(3 * time.Second)
	syscall.Kill(process.Pid, syscall.SIGINT)

	// Waits until SIGINT is sent and responded to, then kills process to avoid timeout
	time.Sleep(3 * time.Second)
	process.Kill()

	sigIntRecieved := strings.Contains(result.Stdout(), testutil.SigProxyTrueOut)
	timedOut := strings.Contains(result.Stdout(), testutil.SigProxyTimeoutMsg)

	return result.Stdout(), sigIntRecieved, timedOut
}

func TestRunSigProxy(t *testing.T) {

	type testCase struct {
		name        string
		args        []string
		want        bool
		expectedOut string
	}
	testCases := []testCase{
		{
			name:        "SigProxyDefault",
			args:        []string{},
			want:        true,
			expectedOut: testutil.SigProxyTrueOut,
		},
		{
			name:        "SigProxyTrue",
			args:        []string{"--sig-proxy=true"},
			want:        true,
			expectedOut: testutil.SigProxyTrueOut,
		},
		{
			name:        "SigProxyFalse",
			args:        []string{"--sig-proxy=false"},
			want:        false,
			expectedOut: "",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			stdout, sigIntRecieved, timedOut := runSigProxy(t, tc.args...)
			errorMsg := fmt.Sprintf("%s failed;\nExpected: '%s'\nActual: '%s'", tc.name, tc.expectedOut, stdout)
			assert.Equal(t, false, timedOut, errorMsg)
			assert.Equal(t, tc.want, sigIntRecieved, errorMsg)
		})
	}
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

func TestRunWithOOMScoreAdj(t *testing.T) {
	if rootlessutil.IsRootless() {
		t.Skip("test skipped for rootless containers.")
	}
	t.Parallel()
	base := testutil.NewBase(t)
	var score = "-42"

	base.Cmd("run", "--rm", "--oom-score-adj", score, testutil.AlpineImage, "cat", "/proc/self/oom_score_adj").AssertOutContains(score)
}

func TestRunWithDetachKeys(t *testing.T) {
	t.Parallel()

	if testutil.GetTarget() == testutil.Docker {
		t.Skip("When detaching from a container, for a session started with 'docker attach'" +
			", it prints 'read escape sequence', but for one started with 'docker (run|start)', it prints nothing." +
			" However, the flag is called '--detach-keys' in all cases" +
			", so nerdctl prints 'read detach keys' for all cases" +
			", and that's why this test is skipped for Docker.")
	}

	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)
	opts := []func(*testutil.Cmd){
		testutil.WithStdin(testutil.NewDelayOnceReader(bytes.NewReader([]byte{1, 2}))), // https://www.physics.udel.edu/~watson/scen103/ascii.html
	}
	defer base.Cmd("container", "rm", "-f", containerName).AssertOK()
	// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
	// unbuffer(1) can be installed with `apt-get install expect`.
	//
	// "-p" is needed because we need unbuffer to read from stdin, and from [1]:
	// "Normally, unbuffer does not read from stdin. This simplifies use of unbuffer in some situations.
	//  To use unbuffer in a pipeline, use the -p flag."
	//
	// [1] https://linux.die.net/man/1/unbuffer
	base.CmdWithHelper([]string{"unbuffer", "-p"}, "run", "-it", "--detach-keys=ctrl-a,ctrl-b", "--name", containerName, testutil.CommonImage).
		CmdOption(opts...).AssertOutContains("read detach keys")
	container := base.InspectContainer(containerName)
	assert.Equal(base.T, container.State.Running, true)
}
