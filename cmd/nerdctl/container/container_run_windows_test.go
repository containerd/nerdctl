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
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestRunHostProcessContainer(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Docker)
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		hostname, err := exec.Command("hostname").Output()
		if err != nil {
			t.Fatalf("unable to get hostname: %s", err)
		}
		data.Labels().Set("hostname", string(bytes.TrimSpace(hostname)))

		whoami := helpers.Capture("run", "--rm", "--isolation=host", testutil.WindowsNano, "whoami")
		t.Logf("whoami %s", whoami)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "--rm", "--isolation=host", testutil.WindowsNano, "hostname")
	}
	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return test.Expects(expect.ExitCodeSuccess, nil, expect.Contains(data.Labels().Get("hostname")))(data, helpers)
	}
	testCase.Run(t)
}

func TestRunHostProcessContainerAsUser(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Docker)
	testCase.Command = test.Command("run", "--rm", "--isolation=host", "-u", "NT AUTHORITY\\SYSTEM", testutil.WindowsNano, "whoami")
	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("nt authority\\system"))
	testCase.Run(t)
}

func TestRunHostProcessContainerAsLocalService(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Docker)
	testCase.Command = test.Command("run", "--rm", "--isolation=host", "-u", "NT AUTHORITY\\Local Service", testutil.WindowsNano, "whoami")
	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("nt authority\\local service"))
	testCase.Run(t)
}

func TestRunHostProcessContainerAsNetworkService(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Docker)
	testCase.Command = test.Command("run", "--rm", "--isolation=host", "-u", "NT AUTHORITY\\Network Service", testutil.WindowsNano, "whoami")
	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("nt authority\\network service"))
	testCase.Run(t)
}

func TestRunProcessIsolated(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Docker)
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Labels().Set("containeruser", "ContainerUser")
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "--rm", "--isolation=process", "-u", data.Labels().Get("containeruser"), testutil.WindowsNano, "whoami")
	}
	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return test.Expects(expect.ExitCodeSuccess, nil, expect.Contains(data.Labels().Get("containeruser")))(data, helpers)
	}
	testCase.Run(t)
}

func TestRunHyperVContainer(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.All(
		require.Not(nerdtest.Docker),
		nerdtest.HyperV,
	)
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		// hyperv must not be in the name for this test, the output is parsed for it
		containerName := "nerdctl-testwcowcontainer"
		data.Labels().Set("containerName", containerName)
		helpers.Ensure("run", "--isolation", "hyperv", "--name", containerName, testutil.WindowsNano)
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Labels().Get("containerName"))
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("container", "inspect", "--mode", "native", data.Labels().Get("containerName"))
	}
	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("hyperv"))(data, helpers)
	}
	testCase.Run(t)
}

func TestRunProcessContainer(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Docker)
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "--isolation", "process", "--name", data.Identifier(), testutil.WindowsNano)
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("container", "inspect", "--mode", "native", data.Identifier())
	}
	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.DoesNotContain("hyperv"))
	testCase.Run(t)
}

// Note that the current implementation of this test is not ideal, since it relies on internal HCS details that
// Microsoft could decide to change in the future (breaking both this unit test and the one in containerd itself):
// https://github.com/containerd/containerd/pull/6618#discussion_r823302852
func TestRunProcessContainerWithDevice(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Docker)
	testCase.Command = test.Command(
		"run",
		"--rm",
		"--isolation=process",
		"--device", "class://5B45201D-F2F2-4F3B-85BB-30FF1F953599",
		testutil.WindowsNano,
		"cmd", "/S", "/C", "dir C:\\Windows\\System32\\HostDriverStore",
	)
	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("FileRepository"))
	testCase.Run(t)
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

	testCase.Expected = test.Expects(0, nil, expect.Contains("Hello, World with TTY!"))

	testCase.Run(t)
}
