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
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestRunHostProcessContainer(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	hostname, err := exec.Command("hostname").Output()
	if err != nil {
		t.Fatalf("unable to get hostname: %s", err)
	}
	hostname = bytes.TrimSpace(hostname)

	base.Cmd("run", "--rm", "--isolation=host", testutil.WindowsNano, "hostname").AssertOutContains(string(hostname))
	output := base.Cmd("run", "--rm", "--isolation=host", testutil.WindowsNano, "whoami").Out()
	t.Logf("whoami %s", output)
}

func TestRunHostProcessContainerAsUser(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)

	hostuser := "nt authority\\system"
	base.Cmd("run", "--rm", "--isolation=host", "-u", "NT AUTHORITY\\SYSTEM", testutil.WindowsNano, "whoami").AssertOutContains(hostuser)
}

func TestRunHostProcessContainerAsService(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	hostuser := "nt authority\\local service"
	base.Cmd("run", "--rm", "--isolation=host", "-u", "NT AUTHORITY\\Local Service", testutil.WindowsNano, "whoami").AssertOutContains(hostuser)
}

func TestRunHostProcessContainerAslocalService(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	hostuser := "nt authority\\network service"
	base.Cmd("run", "--rm", "--isolation=host", "-u", "NT AUTHORITY\\Network Service", testutil.WindowsNano, "whoami").AssertOutContains(hostuser)
}

func TestRunProcessIsolated(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)

	containerUser := "ContainerUser"
	base.Cmd("run", "--rm", "--isolation=process", "-u", containerUser, testutil.WindowsNano, "whoami").AssertOutContains(containerUser)
}

func TestRunHyperVContainer(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)

	if !testutil.HyperVSupported() {
		t.Skip("HyperV is not enabled, skipping test")
	}

	// hyperv must not be in the name for this test, the output is parsed for it
	containerName := "nerdctl-testwcowcontainer"
	base.Cmd("run", "--isolation", "hyperv", "--name", containerName, testutil.WindowsNano).Out()
	defer base.Cmd("rm", "-f", containerName).AssertOK()
	inspectOutput := base.Cmd("container", "inspect", "--mode", "native", containerName).Out()

	assert.Assert(t, strings.Contains(inspectOutput, "hyperv"))
}

func TestRunProcessContainer(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)

	base.Cmd("run", "--isolation", "process", "--name", containerName, testutil.WindowsNano).Out()
	defer base.Cmd("rm", "-f", containerName).AssertOK()
	inspectOutput := base.Cmd("container", "inspect", "--mode", "native", containerName).Out()
	t.Log(inspectOutput)

	assert.Assert(t, !strings.Contains(inspectOutput, "hyperv"))
}

// Note that the current implementation of this test is not ideal, since it relies on internal HCS details that
// Microsoft could decide to change in the future (breaking both this unit test and the one in containerd itself):
// https://github.com/containerd/containerd/pull/6618#discussion_r823302852
func TestRunProcessContainerWithDevice(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)

	base.Cmd(
		"run",
		"--rm",
		"--isolation=process",
		"--device", "class://5B45201D-F2F2-4F3B-85BB-30FF1F953599",
		testutil.WindowsNano,
		"cmd", "/S", "/C", "dir C:\\Windows\\System32\\HostDriverStore",
	).AssertOutContains("FileRepository")
}

func TestRunWithTtyAndDetached(t *testing.T) {
	testCase := nerdtest.Setup()

	// This test is currently disabled, as it is failing most of the time.
	testCase.Require = nerdtest.NerdctlNeedsFixing("https://github.com/containerd/nerdctl/issues/3437")

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		// with -t, success, the container should run with tty support.
		helpers.Ensure("run", "-d", "-t", "--name", data.Identifier("with-terminal"), testutil.CommonImage, "cmd", "/c", "echo", "Hello, World with TTY!")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("container", "rm", "-f", data.Identifier("with-terminal"))
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		withTtyContainer := nerdtest.InspectContainer(helpers, data.Identifier("with-terminal"))
		assert.Equal(helpers.T(), 0, withTtyContainer.State.ExitCode)
		return helpers.Command("logs", data.Identifier("with-terminal"))
	}

	testCase.Expected = test.Expects(0, nil, test.Contains("Hello, World with TTY!"))

	testCase.Run(t)
}
