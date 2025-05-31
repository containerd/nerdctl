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
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

// iptablesCheckCommand is the shell command to check iptables rules
const iptablesCheckCommand = "iptables -t nat -S && iptables -t filter -S && iptables -t mangle -S"

// testContainerRmIptablesExecutor is a common executor function for testing iptables rules cleanup
func testContainerRmIptablesExecutor(data test.Data, helpers test.Helpers) test.TestableCommand {
	t := helpers.T()

	// Get the actual container ID from the container name and store it in a label
	containerID := data.Labels().Get("containerID")
	if containerID == "" {
		container := nerdtest.InspectContainer(helpers, data.Identifier())
		containerID = container.ID
		data.Labels().Set("containerID", containerID)
	}

	// Remove the container
	helpers.Ensure("rm", "-f", containerID)

	time.Sleep(1 * time.Second)

	// Create a TestableCommand using helpers.Custom
	if rootlessutil.IsRootless() {
		// In rootless mode, we need to enter the rootlesskit network namespace
		stateDir, err := rootlessutil.RootlessKitStateDir()
		if err != nil {
			t.Fatalf("Failed to get rootlesskit state dir: %v", err)
		}

		childPid, err := rootlessutil.RootlessKitChildPid(stateDir)
		if err != nil {
			t.Fatalf("Failed to get rootlesskit child pid: %v", err)
		}

		// Construct the path to the network namespace
		uid := os.Getuid()
		netnsPath := fmt.Sprintf("/run/user/%d/containerd-rootless/netns", uid)

		if netns, err := rootlessutil.DetachedNetNS(); err != nil {
			t.Fatalf("Failed to get detached network namespace: %v", err)
		} else {
			if netns != "" {
				// First enter the user and mount namespace, then the network namespace
				return helpers.Custom("nsenter", "-t", strconv.Itoa(childPid), "-U", "-m", "--preserve-credentials",
					"--", "sh", "-c", fmt.Sprintf("nsenter --net=%s sh -c '%s'", netnsPath, iptablesCheckCommand))
			}
			// Enter into RootlessKit namespace using containerd-rootless-setuptool.sh
			return helpers.Custom("containerd-rootless-setuptool.sh", "nsenter", "--", "sh", "-c", iptablesCheckCommand)
		}
	}

	// In non-rootless mode, check iptables rules directly on the host
	return helpers.Custom("sh", "-c", iptablesCheckCommand)
}

// TestContainerRmIptables tests that iptables rules are cleared after container deletion
func TestContainerRmIptables(t *testing.T) {
	testCase := nerdtest.Setup()

	// Require iptables and containerd-rootless-setuptool.sh commands to be available
	testCase.Require = require.All(
		require.Binary("iptables"),
		require.Binary("containerd-rootless-setuptool.sh"),
		require.Not(require.Windows),
		require.Not(nerdtest.Docker),
	)

	testCase.SubTests = []*test.Case{
		{
			Description: "Test iptables rules are cleared after container deletion",
			Setup: func(data test.Data, helpers test.Helpers) {
				// Create a container with port mapping to ensure iptables rules are created
				helpers.Ensure("run", "-d", "--name", data.Identifier(), "-p", "8080:80", testutil.NginxAlpineImage)
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				// Make sure container is removed even if test fails
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: testContainerRmIptablesExecutor,
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				// Get the container ID from the label
				containerID := data.Labels().Get("containerID")
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					// Verify that the iptables output does not contain the container ID
					Output: expect.DoesNotContain(containerID),
				}
			},
		},
	}

	testCase.Run(t)
}
