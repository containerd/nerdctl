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
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestRestart(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)

	base.Cmd("run", "-d", "--name", tID, testutil.NginxAlpineImage).AssertOK()
	defer base.Cmd("rm", "-f", tID).AssertOK()
	base.EnsureContainerStarted(tID)

	inspect := base.InspectContainer(tID)
	pid := inspect.State.Pid

	base.Cmd("restart", tID).AssertOK()
	base.EnsureContainerStarted(tID)

	newInspect := base.InspectContainer(tID)
	newPid := newInspect.State.Pid
	assert.Assert(t, pid != newPid)
}

func TestRestartPIDContainer(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)

	baseContainerName := testutil.Identifier(t)
	base.Cmd("run", "-d", "--name", baseContainerName, testutil.AlpineImage, "sleep", nerdtest.Infinity).AssertOK()
	defer base.Cmd("rm", "-f", baseContainerName).Run()

	sharedContainerName := fmt.Sprintf("%s-shared", baseContainerName)
	base.Cmd("run", "-d", "--name", sharedContainerName, fmt.Sprintf("--pid=container:%s", baseContainerName), testutil.AlpineImage, "sleep", nerdtest.Infinity).AssertOK()
	defer base.Cmd("rm", "-f", sharedContainerName).Run()

	base.Cmd("restart", baseContainerName).AssertOK()
	base.Cmd("restart", sharedContainerName).AssertOK()

	// output format : <inode number> /proc/1/ns/pid
	// example output: 4026532581 /proc/1/ns/pid
	basePSResult := base.Cmd("exec", baseContainerName, "ls", "-Li", "/proc/1/ns/pid").Run()
	baseOutput := strings.TrimSpace(basePSResult.Stdout())
	sharedPSResult := base.Cmd("exec", sharedContainerName, "ls", "-Li", "/proc/1/ns/pid").Run()
	sharedOutput := strings.TrimSpace(sharedPSResult.Stdout())

	assert.Equal(t, baseOutput, sharedOutput)
}

func TestRestartIPCContainer(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)

	const shmSize = "32m"
	baseContainerName := testutil.Identifier(t)
	defer base.Cmd("rm", "-f", baseContainerName).Run()
	base.Cmd("run", "-d", "--shm-size", shmSize, "--ipc", "shareable", "--name", baseContainerName, testutil.AlpineImage, "sleep", nerdtest.Infinity).AssertOK()

	sharedContainerName := fmt.Sprintf("%s-shared", baseContainerName)
	defer base.Cmd("rm", "-f", sharedContainerName).Run()
	base.Cmd("run", "-d", "--name", sharedContainerName, fmt.Sprintf("--ipc=container:%s", baseContainerName), testutil.AlpineImage, "sleep", nerdtest.Infinity).AssertOK()

	base.Cmd("stop", baseContainerName).Run()
	base.Cmd("stop", sharedContainerName).Run()

	base.Cmd("restart", baseContainerName).AssertOK()
	base.Cmd("restart", sharedContainerName).AssertOK()

	baseShmSizeResult := base.Cmd("exec", baseContainerName, "/bin/grep", "shm", "/proc/self/mounts").Run()
	baseOutput := strings.TrimSpace(baseShmSizeResult.Stdout())
	sharedShmSizeResult := base.Cmd("exec", sharedContainerName, "/bin/grep", "shm", "/proc/self/mounts").Run()
	sharedOutput := strings.TrimSpace(sharedShmSizeResult.Stdout())

	assert.Equal(t, baseOutput, sharedOutput)
}

func TestRestartWithTime(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)

	base.Cmd("run", "-d", "--name", tID, testutil.AlpineImage, "sleep", nerdtest.Infinity).AssertOK()
	defer base.Cmd("rm", "-f", tID).AssertOK()

	inspect := base.InspectContainer(tID)
	pid := inspect.State.Pid

	timePreRestart := time.Now()
	base.Cmd("restart", "-t", "5", tID).AssertOK()
	timePostRestart := time.Now()

	newInspect := base.InspectContainer(tID)
	newPid := newInspect.State.Pid
	assert.Assert(t, pid != newPid)
	// ensure that stop took at least 5 seconds
	assert.Assert(t, timePostRestart.Sub(timePreRestart) >= time.Second*5)
}

func TestRestartWithSignal(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)

	base.Cmd("run", "-d", "--name", tID, testutil.AlpineImage, "sh", "-c", `
		trap 'echo "Received SIGUSR1"; exit 0' SIGUSR1
		echo "Starting"
		while true; do
			sleep 1
		done
	`).AssertOK()
	defer base.Cmd("rm", "-f", tID).Run()

	base.EnsureContainerStarted(tID)

	inspect := base.InspectContainer(tID)
	initialPid := inspect.State.Pid

	base.Cmd("restart", "--signal", "SIGUSR1", tID).AssertOK()
	base.EnsureContainerStarted(tID)

	newInspect := base.InspectContainer(tID)
	newPid := newInspect.State.Pid

	assert.Assert(t, initialPid != newPid, "Container PID should have changed after restart")

}
