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
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	cniutils "github.com/containernetworking/plugins/pkg/utils"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/portlock"
)

// iptablesCheckCommand is the shell command to check iptables rules
const iptablesCheckCommand = "iptables -t nat -S && iptables -t filter -S && iptables -t mangle -S"

// testContainerRmIptablesExecutor is a common executor function for testing iptables rules cleanup
func testContainerRmIptablesExecutor(data test.Data, helpers test.Helpers) test.TestableCommand {
	t := helpers.T()

	// Get the container ID from the label
	containerID := data.Labels().Get("containerID")

	// Remove the container
	helpers.Ensure("rm", "-f", containerID)

	time.Sleep(1 * time.Second)

	// Create a TestableCommand using helpers.Custom
	if rootlessutil.IsRootless() {
		// In rootless mode, we need to enter the rootlesskit network namespace
		if netns, err := rootlessutil.DetachedNetNS(); err != nil {
			t.Log(fmt.Sprintf("Failed to get detached network namespace: %v", err))
			t.FailNow()
		} else {
			if netns != "" {
				// Use containerd-rootless-setuptool.sh to enter the RootlessKit namespace
				return helpers.Custom("containerd-rootless-setuptool.sh", "nsenter", "--", "nsenter", "--net="+netns, "sh", "-ec", iptablesCheckCommand)
			}
			// Enter into :RootlessKit namespace using containerd-rootless-setuptool.sh
			return helpers.Custom("containerd-rootless-setuptool.sh", "nsenter", "--", "sh", "-ec", iptablesCheckCommand)
		}
	}

	// In non-rootless mode, check iptables rules directly on the host
	return helpers.Custom("sh", "-ec", iptablesCheckCommand)
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
				// Get a free port using portlock
				port, err := portlock.Acquire(0)
				if err != nil {
					helpers.T().Log(fmt.Sprintf("Failed to acquire port: %v", err))
					helpers.T().FailNow()
				}
				data.Labels().Set("port", strconv.Itoa(port))

				// Create a container with port mapping to ensure iptables rules are created
				containerID := helpers.Capture("run", "-d", "--name", data.Identifier(), "-p", fmt.Sprintf("%d:80", port), testutil.NginxAlpineImage)
				data.Labels().Set("containerID", strings.TrimSpace(containerID))
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				// Make sure container is removed even if test fails
				helpers.Anyhow("rm", "-f", data.Identifier())

				// Release the acquired port
				if portStr := data.Labels().Get("port"); portStr != "" {
					port, _ := strconv.Atoi(portStr)
					_ = portlock.Release(port)
				}
			},
			Command: testContainerRmIptablesExecutor,
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				// Get the container ID from the label
				containerID := data.Labels().Get("containerID")
				id := fmt.Sprintf("%s-%s", helpers.Read(nerdtest.Namespace), containerID)
				chain := cniutils.FormatChainName("bridge", id)
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					// Verify that the iptables output does not contain the container ID and a chain name generated from the cni name, namespace, and container ID
					Output: expect.All(
						expect.DoesNotContain(containerID),
						expect.DoesNotContain(chain),
					),
				}
			},
		},
	}

	testCase.Run(t)
}

func TestRemoveContainerWithoutNetworkAnnotation(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = nerdtest.OnlyKubernetes

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		identifier := data.Identifier()
		namespace := identifier
		containerID := ""

		kubectlPath, _ := exec.LookPath("kubectl")

		createNamespace := helpers.Custom(kubectlPath)
		createNamespace.WithArgs("create", "namespace", namespace)
		createNamespace.Run(&test.Expected{})

		kube := func(args ...string) test.TestableCommand {
			cmd := helpers.Custom(kubectlPath)
			cmd.WithArgs("--namespace=" + namespace)
			cmd.WithArgs(args...)
			return cmd
		}

		// Kubernetes creates containers through CRI in the k8s.io containerd
		// namespace. These containers do not have nerdctl-specific network
		// annotations, which reproduces the scenario from #5207.
		kube(
			"run",
			"--restart=Never",
			"--image",
			testutil.CommonImage,
			identifier,
			"--",
			"sleep",
			nerdtest.Infinity,
		).Run(&test.Expected{})

		cmd := kube(
			"wait",
			"pod",
			identifier,
			"--for=condition=ready",
			"--timeout=1m",
		)
		cmd.WithTimeout(70 * time.Second)
		cmd.Run(&test.Expected{})

		// Retrieve the actual containerd container ID created by Kubernetes.
		kube(
			"get",
			"pods",
			identifier,
			"-o",
			"jsonpath={ .status.containerStatuses[0].containerID }",
		).Run(&test.Expected{
			Output: func(stdout string, t tig.T) {
				containerID = strings.TrimPrefix(stdout, "containerd://")
			},
		})

		data.Labels().Set("containerID", containerID)

		// Stop the CRI-created container without deleting it from containerd.
		// nerdctl rm must still succeed even without nerdctl network metadata.
		helpers.Ensure("stop", containerID)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		kubectlPath, err := exec.LookPath("kubectl")
		if err != nil {
			return
		}

		cmd := helpers.Custom(kubectlPath)
		cmd.WithArgs("delete", "namespace", data.Identifier(), "--ignore-not-found=true")
		cmd.WithTimeout(30 * time.Second)
		cmd.Run(nil)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("rm", data.Labels().Get("containerID"))
	}

	testCase.Expected = test.Expects(0, nil, nil)

	testCase.Run(t)
}
