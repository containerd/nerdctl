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
	"strconv"
	"strings"
	"testing"

	"github.com/coreos/go-iptables/iptables"
	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	iptablesutil "github.com/containerd/nerdctl/v2/pkg/testutil/iptables"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/portlock"
)

// TestKillCleanupForwards runs a container that exposes a port and then kills it.
// The test checks that the kill command effectively cleans up
// the iptables forwards created from the run.
func TestKillCleanupForwards(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = nerdtest.Rootful // pkg/testutil/iptables does not support rootless

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		port, err := portlock.Acquire(0)
		assert.NilError(helpers.T(), err)
		data.Labels().Set("hostPort", strconv.Itoa(port))

		containerID := helpers.Capture("run", "-d",
			"--restart=no",
			"--name", data.Identifier(),
			"-p", fmt.Sprintf("127.0.0.1:%d:80", port),
			testutil.NginxAlpineImage)
		containerID = strings.TrimSuffix(containerID, "\n")

		containerIP := helpers.Capture("inspect",
			"-f",
			"'{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}'",
			data.Identifier())
		containerIP = strings.ReplaceAll(containerIP, "'", "")
		containerIP = strings.TrimSuffix(containerIP, "\n")

		// define iptables chain name depending on the target (docker/nerdctl)
		ipt, err := iptables.New()
		assert.NilError(helpers.T(), err)

		var chain string
		if nerdtest.IsDocker() {
			chain = "DOCKER"
		} else {
			redirectChain := "CNI-HOSTPORT-DNAT"
			chain = iptablesutil.GetRedirectedChain(t, ipt, redirectChain, testutil.Namespace, containerID)
		}

		data.Labels().Set("chain", chain)
		data.Labels().Set("containerIP", containerIP)
		data.Labels().Set("containerName", data.Identifier())
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Labels().Get("containerName"))
		if portStr := data.Labels().Get("hostPort"); portStr != "" {
			port, err := strconv.Atoi(portStr)
			if err == nil {
				_ = portlock.Release(port)
			}
		}
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "iptables forwarding rule should exist before container is killed",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Custom("iptables", "-t", "nat", "-S", data.Labels().Get("chain"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						rules := strings.Split(stdout, "\n")
						port, _ := strconv.Atoi(data.Labels().Get("hostPort"))
						found, err := iptablesutil.ForwardExistsFromRules(rules, data.Labels().Get("containerIP"), port)
						assert.NilError(t, err)
						assert.Assert(t, found, "iptables forwarding rule should exist before kill")
					},
				}
			},
		},
		{
			Description: "kill container",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("kill", data.Labels().Get("containerName"))
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
		{
			Description: "iptables forwarding rule should be removed after container is killed",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Custom("iptables", "-t", "nat", "-S", data.Labels().Get("chain"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeNoCheck,
					Output: func(stdout string, t tig.T) {
						rules := strings.Split(stdout, "\n")
						port, _ := strconv.Atoi(data.Labels().Get("hostPort"))
						found, err := iptablesutil.ForwardExistsFromRules(rules, data.Labels().Get("containerIP"), port)
						if err != nil {
							// chain may have been removed entirely after kill — that's fine
							return
						}
						assert.Assert(t, !found, "iptables forwarding rule should be removed after kill")
					},
				}
			},
		},
	}

	testCase.Run(t)
}
