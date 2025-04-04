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
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
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
	base.Cmd("pull", "--quiet", imageName).AssertOK()
	tmpDir, err := os.MkdirTemp(base.T.TempDir(), "test-save")
	assert.NilError(base.T, err)
	defer os.RemoveAll(tmpDir)
	archiveTarPath := filepath.Join(tmpDir, "a.tar")
	base.Cmd("save", "-o", archiveTarPath, imageName).AssertOK()
	rootfs, err := os.MkdirTemp(base.T.TempDir(), "rootfs")
	assert.NilError(base.T, err)
	err = helpers.ExtractDockerArchive(archiveTarPath, rootfs)
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
	result := base.Cmd("run", "--name", container, "--ipc", "container:abcd1234", testutil.AlpineImage, "sleep", nerdtest.Infinity).Run()
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
	sharedContainerResult := base.Cmd("run", "-d", "--ipc", "shareable", "--shm-size", shmSize, testutil.AlpineImage, "sleep", nerdtest.Infinity).Run()
	baseContainerID := strings.TrimSpace(sharedContainerResult.Stdout())
	defer base.Cmd("rm", "-f", baseContainerID).Run()

	base.Cmd("run", "--rm", fmt.Sprintf("--ipc=container:%s", baseContainerID),
		testutil.AlpineImage, "/bin/grep", "shm", "/proc/self/mounts").AssertOutContains("size=32768k")
}

func TestRunIPCContainer(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)

	const shmSize = "32m"
	victimContainerResult := base.Cmd("run", "-d", "--ipc", "shareable", "--shm-size", shmSize, testutil.AlpineImage, "sleep", nerdtest.Infinity).Run()
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
	// Validate we can't provide a domainname with uts=host
	base.Cmd("run", "--rm", "--uts=host", "--domainname=example.com", testutil.AlpineImage, "hostname").AssertFail()
}

func TestRunPidContainer(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)

	sharedContainerResult := base.Cmd("run", "-d", testutil.AlpineImage, "sleep", nerdtest.Infinity).Run()
	baseContainerID := strings.TrimSpace(sharedContainerResult.Stdout())
	defer base.Cmd("rm", "-f", baseContainerID).Run()

	base.Cmd("run", "--rm", fmt.Sprintf("--pid=container:%s", baseContainerID),
		testutil.AlpineImage, "ps", "ax").AssertOutContains("sleep " + nerdtest.Infinity)
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
	testutil.RequireExecutable(t, "tini-custom")
	base := testutil.NewBase(t)

	container := testutil.Identifier(t)
	base.Cmd("run", "-d", "--name", container, testutil.AlpineImage, "sleep", nerdtest.Infinity).AssertOK()
	defer base.Cmd("rm", "-f", container).Run()

	base.Cmd("stop", "--time=3", container).AssertOK()
	// Unable to handle TERM signal, be killed when timeout
	assert.Equal(t, base.InspectContainer(container).State.ExitCode, 137)

	// Test with --init-path
	container1 := container + "-1"
	base.Cmd("run", "-d", "--name", container1, "--init-binary", "tini-custom",
		testutil.AlpineImage, "sleep", nerdtest.Infinity).AssertOK()
	defer base.Cmd("rm", "-f", container1).Run()

	base.Cmd("stop", "--time=3", container1).AssertOK()
	assert.Equal(t, base.InspectContainer(container1).State.ExitCode, 143)

	// Test with --init
	container2 := container + "-2"
	base.Cmd("run", "-d", "--name", container2, "--init",
		testutil.AlpineImage, "sleep", nerdtest.Infinity).AssertOK()
	defer base.Cmd("rm", "-f", container2).Run()

	base.Cmd("stop", "--time=3", container2).AssertOK()
	assert.Equal(t, base.InspectContainer(container2).State.ExitCode, 143)
}

func TestRunTTY(t *testing.T) {
	const sttyPartialOutput = "speed 38400 baud"

	testCase := nerdtest.Setup()

	testCase.SubTests = []*test.Case{
		{
			Description: "stty with -it",
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("run", "-it", data.Identifier(), "stty")
				cmd.WithPseudoTTY()
				return cmd
			},
			Expected: test.Expects(0, nil, expect.Contains(sttyPartialOutput)),
		},
		{
			Description: "stty with -t",
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("run", "-t", data.Identifier(), "stty")
				cmd.WithPseudoTTY()
				return cmd
			},
			Expected: test.Expects(0, nil, expect.Contains(sttyPartialOutput)),
		},
		{
			Description: "stty with -i",
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("run", "-i", data.Identifier(), "stty")
				cmd.WithPseudoTTY()
				return cmd
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			Description: "stty without params",
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("run", data.Identifier(), "stty")
				cmd.WithPseudoTTY()
				return cmd
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			Description: "stty with -td",
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("run", "-td", data.Identifier(), "stty")
				cmd.WithPseudoTTY()
				return cmd
			},
			Expected: test.Expects(0, nil, nil),
		},
	}
}

func TestRunSigProxy(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.SubTests = []*test.Case{
		{
			Description: "SigProxyDefault",

			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},

			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				// FIXME: os.Interrupt will likely not work on Windows
				cmd := nerdtest.RunSigProxyContainer(os.Interrupt, true, nil, data, helpers)
				err := cmd.Signal(os.Interrupt)
				assert.NilError(helpers.T(), err)
				return cmd
			},

			Expected: test.Expects(0, nil, expect.Contains(nerdtest.SignalCaught)),
		},
		{
			Description: "SigProxyTrue",

			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},

			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := nerdtest.RunSigProxyContainer(os.Interrupt, true, []string{"--sig-proxy=true"}, data, helpers)
				err := cmd.Signal(os.Interrupt)
				assert.NilError(helpers.T(), err)
				return cmd
			},

			Expected: test.Expects(0, nil, expect.Contains(nerdtest.SignalCaught)),
		},
		{
			Description: "SigProxyFalse",

			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},

			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := nerdtest.RunSigProxyContainer(os.Interrupt, true, []string{"--sig-proxy=false"}, data, helpers)
				err := cmd.Signal(os.Interrupt)
				assert.NilError(helpers.T(), err)
				return cmd
			},

			Expected: test.Expects(expect.ExitCodeSignaled, nil, expect.DoesNotContain(nerdtest.SignalCaught)),
		},
	}

	testCase.Run(t)
}

func TestRunWithFluentdLogDriver(t *testing.T) {
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
	testCase := nerdtest.Setup()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		// Run interactively and detach
		cmd := helpers.Command("run", "-it", "--detach-keys=ctrl-a,ctrl-b", "--name", data.Identifier(), testutil.CommonImage)
		cmd.WithPseudoTTY()
		cmd.Feed(strings.NewReader("echo mark${NON}mark\n"))
		cmd.WithFeeder(func() io.Reader {
			// Because of the way we proxy stdin, we have to wait here, otherwise we detach before
			// the rest of the input ever reaches the container
			// Note that this only concerns nerdctl, as docker seems to behave ok LOCALLY.
			// But then, it fails for docker as well ON THE CI. It is unclear why at this point.
			// Arbitrary time pauses would not work: what matters is that the container has started.
			// if !nerdtest.IsDocker() {
			nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			// }
			// ctrl+a and ctrl+b (see https://en.wikipedia.org/wiki/C0_and_C1_control_codes)
			return bytes.NewReader([]byte{1, 2})
		})

		return cmd
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0,
			Errors:   []error{errors.New("detach keys")},
			Output: expect.All(
				expect.Contains("markmark"),
				func(stdout string, info string, t *testing.T) {
					assert.Assert(t, strings.Contains(helpers.Capture("inspect", "--format", "json", data.Identifier()), "\"Running\":true"))
				},
			),
		}
	}

	testCase.Run(t)
}

func TestRunWithTtyAndDetached(t *testing.T) {
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

// TestIssue3568 tests https://github.com/containerd/nerdctl/issues/3568
func TestIssue3568(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Description = "Issue #3568 - Detaching from a container started by using --rm option causes the container to be deleted."

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		// Run interactively and detach
		cmd := helpers.Command("run", "--rm", "-it", "--detach-keys=ctrl-a,ctrl-b", "--name", data.Identifier(), testutil.CommonImage)
		cmd.WithPseudoTTY()
		cmd.Feed(strings.NewReader("echo mark${NON}mark\n"))
		cmd.WithFeeder(func() io.Reader {
			// Because of the way we proxy stdin, we have to wait here, otherwise we detach before
			// the rest of the input ever reaches the container
			// Note that this only concerns nerdctl, as docker seems to behave ok LOCALLY.
			// But then, it fails for docker as well ON THE CI. It is unclear why at this point.
			// Arbitrary time pauses would not work: what matters is that the container has started.
			// if !nerdtest.IsDocker() {
			nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			// }
			// ctrl+a and ctrl+b (see https://en.wikipedia.org/wiki/C0_and_C1_control_codes)
			return bytes.NewReader([]byte{1, 2})
		})

		return cmd
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0,
			Errors:   []error{errors.New("detach keys")},
			Output: expect.All(
				expect.Contains("markmark"),
				func(stdout string, info string, t *testing.T) {
					assert.Assert(t, strings.Contains(helpers.Capture("inspect", "--format", "json", data.Identifier()), "\"Running\":true"))
				},
			),
		}
	}

	testCase.Run(t)
}

// TestPortBindingWithCustomHost tests https://github.com/containerd/nerdctl/issues/3539
func TestPortBindingWithCustomHost(t *testing.T) {
	testCase := nerdtest.Setup()

	const (
		host     = "127.0.0.2"
		hostPort = 8080
	)
	address := fmt.Sprintf("%s:%d", host, hostPort)

	testCase.SubTests = []*test.Case{
		{
			Description: "Issue #3539 - Access to a container running when 127.0.0.2 is specified in -p in rootless mode.",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(), "-p", fmt.Sprintf("%s:80", address), testutil.NginxAlpineImage)
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Errors:   []error{},
					Output: expect.All(
						func(stdout string, info string, t *testing.T) {
							resp, err := nettestutil.HTTPGet(address, 30, false)
							assert.NilError(t, err)

							respBody, err := io.ReadAll(resp.Body)
							assert.NilError(t, err)
							assert.Assert(t, strings.Contains(string(respBody), testutil.NginxAlpineIndexHTMLSnippet))
						},
					),
				}
			},
		},
	}

	testCase.Run(t)
}
