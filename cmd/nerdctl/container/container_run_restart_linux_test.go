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
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/containerd/containerd/v2/core/runtime/restart"
	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/portlock"
)

func daemonSystemctlTarget() string {
	if nerdtest.IsDocker() {
		return "docker.service"
	}
	return "containerd.service"
}

func daemonSystemctlArgs() []string {
	if os.Geteuid() != 0 {
		return []string{"--user"}
	}
	return nil
}

func killDaemon(t tig.T) {
	t.Helper()
	target := daemonSystemctlTarget()
	t.Log(fmt.Sprintf("killing %q", target))
	cmd := exec.Command("systemctl", append(daemonSystemctlArgs(), "kill", target)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Log(fmt.Sprintf("cannot kill %q: %q: %v", target, string(out), err))
		t.FailNow()
	}
	// the daemon should restart automatically
}

func ensureDaemonActive(t tig.T) {
	t.Helper()
	target := daemonSystemctlTarget()
	t.Log(fmt.Sprintf("checking activity of %q", target))
	const (
		maxRetry = 30
		sleep    = 3 * time.Second
	)
	for i := 0; i < maxRetry; i++ {
		cmd := exec.Command("systemctl", append(daemonSystemctlArgs(), "is-active", target)...)
		out, err := cmd.CombinedOutput()
		t.Log(fmt.Sprintf("(retry=%d) %s", i, string(out)))
		if err == nil {
			// The daemon is now running, but the daemon may still refuse connections to containerd.sock
			t.Log(fmt.Sprintf("daemon %q is now running, checking whether the daemon can handle requests", target))
			infoOut, infoErr := exec.Command(testutil.GetTarget(), "info").CombinedOutput()
			if infoErr == nil {
				t.Log(fmt.Sprintf("daemon %q can now handle requests", target))
				return
			}
			t.Log(fmt.Sprintf("(retry=%d) info failed: %s: %v", i, string(infoOut), infoErr))
		}
		time.Sleep(sleep)
	}
	t.Log(fmt.Sprintf("daemon %q not running?", target))
	t.FailNow()
}

func dumpDaemonLogs(t tig.T, minutes int) {
	t.Helper()
	target := daemonSystemctlTarget()
	cmd := exec.Command("journalctl",
		append(daemonSystemctlArgs(), "-u", target, "--no-pager", "-S", fmt.Sprintf("%d min ago", minutes))...)
	t.Log(fmt.Sprintf("===== %v =====", cmd.Args))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Log(fmt.Sprintf("failed to dump daemon logs: %v", err))
		return
	}
	t.Log(string(out))
	t.Log("==========")
}

// assertRestartCount asserts that `container inspect` reports the expected RestartCount.
func assertRestartCount(expected int) test.Comparator {
	return expect.JSON([]dockercompat.Container{}, func(dc []dockercompat.Container, t tig.T) {
		assert.Equal(t, 1, len(dc))
		assert.Equal(t, dc[0].RestartCount, expected)
	})
}

func TestRunRestart(t *testing.T) {
	if testing.Short() {
		t.Skipf("test is long")
	}
	if !testutil.GetDaemonIsKillable() {
		t.Skip("daemon is not killable (hint: set \"-test.allow-kill-daemon\")")
	}
	t.Log("NOTE: this test may take a while")

	testCase := nerdtest.Setup()
	testCase.NoParallel = true

	httpCheck := func(data test.Data, httpGetRetry int) error {
		hostPort, _ := strconv.Atoi(data.Labels().Get("hostPort"))
		resp, err := nettestutil.HTTPGet(fmt.Sprintf("http://127.0.0.1:%d", hostPort), httpGetRetry, false)
		if err != nil {
			return err
		}
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if !strings.Contains(string(respBody), testutil.NginxAlpineIndexHTMLSnippet) {
			return fmt.Errorf("expected contain %q, got %q",
				testutil.NginxAlpineIndexHTMLSnippet, string(respBody))
		}
		return nil
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		port, err := portlock.Acquire(0)
		if err != nil {
			helpers.T().Log(fmt.Sprintf("failed to acquire port: %v", err))
			helpers.T().FailNow()
		}
		data.Labels().Set("hostPort", strconv.Itoa(port))

		helpers.Ensure("run", "-d",
			"--restart=always",
			"--name", data.Identifier(),
			"-p", fmt.Sprintf("127.0.0.1:%d:80", port),
			testutil.NginxAlpineImage)

		assert.NilError(helpers.T(), httpCheck(data, 5))
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
		port, err := strconv.Atoi(data.Labels().Get("hostPort"))
		if err == nil {
			portlock.Release(port)
		}
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		killDaemon(helpers.T())
		ensureDaemonActive(helpers.T())
		return helpers.Command("ps", "-a")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, t tig.T) {
				t.Log(fmt.Sprintf("initial ps -a: %q", stdout))

				const (
					maxRetry = 30
					sleep    = 3 * time.Second
				)
				for i := 0; i < maxRetry; i++ {
					err := httpCheck(data, 1)
					if err == nil {
						t.Log(fmt.Sprintf("test is passing, after %d retries", i))
						return
					}
					time.Sleep(sleep)
					t.Log(fmt.Sprintf("(retry %d) ps -a: %q", i, helpers.Capture("ps", "-a")))
				}
				dumpDaemonLogs(t, 10)
				t.Log("the container does not seem to be restarted")
				t.FailNow()
			},
		}
	}

	testCase.Run(t)
}

func TestRunRestartWithOnFailure(t *testing.T) {
	testCase := nerdtest.Setup()
	if !nerdtest.IsDocker() {
		testCase.Require = nerdtest.ContainerdPlugin("io.containerd.internal.v1", "restart", []string{"on-failure"})
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--restart=on-failure:2", "--name", data.Identifier(), testutil.AlpineImage, "sh", "-c", "exit 1")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		nerdtest.EnsureContainerExited(helpers, data.Identifier(), -1)
		return helpers.Command("inspect", data.Identifier())
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output:   assertRestartCount(2),
		}
	}

	testCase.Run(t)
}

func TestRunRestartWithUnlessStopped(t *testing.T) {
	testCase := nerdtest.Setup()
	if !nerdtest.IsDocker() {
		testCase.Require = nerdtest.ContainerdPlugin("io.containerd.internal.v1", "restart", []string{"unless-stopped"})
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--restart=unless-stopped", "--name", data.Identifier(), testutil.AlpineImage, "sh", "-c", "exit 1")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		deadline := time.Now().Add(60 * time.Second)
		for time.Now().Before(deadline) {
			inspect := nerdtest.InspectContainer(helpers, data.Identifier())
			if inspect.State != nil && inspect.State.Status == "exited" {
				break
			}
			if inspect.RestartCount == 2 {
				helpers.Ensure("stop", data.Identifier())
			}
			time.Sleep(100 * time.Millisecond)
		}
		return helpers.Command("inspect", data.Identifier())
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output:   assertRestartCount(2),
		}
	}

	testCase.Run(t)
}

func TestUpdateRestartPolicy(t *testing.T) {
	testCase := nerdtest.Setup()
	if !nerdtest.IsDocker() {
		testCase.Require = nerdtest.ContainerdPlugin("io.containerd.internal.v1", "restart", []string{"on-failure"})
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--restart=on-failure:1", "--name", data.Identifier(), testutil.AlpineImage, "sh", "-c", "exit 1")
		helpers.Ensure("update", "--restart=on-failure:2", data.Identifier())
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		nerdtest.EnsureContainerExited(helpers, data.Identifier(), -1)
		return helpers.Command("inspect", data.Identifier())
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output:   assertRestartCount(2),
		}
	}

	testCase.Run(t)
}

// The test is to add a restart policy to a container which has not restart policy before,
// and check it can work correctly.
func TestAddRestartPolicy(t *testing.T) {
	testCase := nerdtest.Setup()
	if !nerdtest.IsDocker() {
		testCase.Require = nerdtest.ContainerdPlugin("io.containerd.internal.v1", "restart", []string{"on-failure"})
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.NginxAlpineImage)
		helpers.Ensure("update", "--restart=on-failure", data.Identifier())
		inspect := nerdtest.InspectContainer(helpers, data.Identifier())
		data.Labels().Set("originalPid", strconv.Itoa(inspect.State.Pid))
		exec.Command("kill", "-9", strconv.Itoa(inspect.State.Pid)).Run()
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		originalPid, _ := strconv.Atoi(data.Labels().Get("originalPid"))
		deadline := time.Now().Add(60 * time.Second)
		for time.Now().Before(deadline) {
			inspect := nerdtest.InspectContainer(helpers, data.Identifier())
			if inspect.State != nil && inspect.State.Status == "running" && inspect.State.Pid != originalPid {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		return helpers.Command("inspect", data.Identifier())
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output:   assertRestartCount(1),
		}
	}

	testCase.Run(t)
}

func TestRunRestartStatusLabel(t *testing.T) {
	testCase := nerdtest.Setup()
	if !nerdtest.IsDocker() {
		testCase.Require = nerdtest.ContainerdPlugin("io.containerd.internal.v1", "restart", []string{"always"})
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("create", "--restart=always", "--name", data.Identifier(), testutil.CommonImage, "sleep", "infinity")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("inspect", data.Identifier())
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: expect.JSON([]dockercompat.Container{}, func(dc []dockercompat.Container, t tig.T) {
				assert.Equal(t, 1, len(dc))
				assert.Assert(t, dc[0].Config.Labels[restart.StatusLabel] == "")
			}),
		}
	}

	testCase.Run(t)
}
