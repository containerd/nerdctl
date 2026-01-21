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
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	iptablesutil "github.com/containerd/nerdctl/v2/pkg/testutil/iptables"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/portlock"
)

// TestKillCleanupForwards runs a container that exposes a port and then kill it.
// The test checks that the kill command effectively clean up
// the iptables forwards created from the run.
func TestKillCleanupForwards(t *testing.T) {

	ipt, err := iptables.New()
	assert.NilError(t, err)

	testCase := nerdtest.Setup()

	testCase.Require = require.Not(nerdtest.Rootless)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		hostPort, err := portlock.Acquire(0)
		if err != nil {
			t.Logf("Failed to acquire port: %v", err)
			t.FailNow()
		}

		containerName := data.Identifier()

		helpers.Ensure(
			"run", "-d",
			"--restart=no",
			"--name", containerName,
			"-p", fmt.Sprintf("127.0.0.1:%d:80", hostPort),
			testutil.NginxAlpineImage,
		)
		nerdtest.EnsureContainerStarted(helpers, containerName)

		containerID := strings.TrimSpace(
			helpers.Capture("inspect", "-f", "{{.Id}}", containerName),
		)

		containerIP := strings.TrimSpace(
			helpers.Capture(
				"inspect",
				"-f", "{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}",
				containerName,
			),
		)

		// define iptables chain name depending on the target (docker/nerdctl)
		var chain string
		if nerdtest.IsDocker() {
			chain = "DOCKER"
		} else {
			chain = iptablesutil.GetRedirectedChain(
				t,
				ipt,
				"CNI-HOSTPORT-DNAT",
				testutil.Namespace,
				containerID,
			)
		}

		data.Labels().Set("containerName", containerName)
		data.Labels().Set("containerIP", containerIP)
		data.Labels().Set("hostPort", strconv.Itoa(hostPort))
		data.Labels().Set("chain", chain)
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
					Output: func(stdout string, t tig.T) {
						rules := strings.Split(stdout, "\n")
						containerIP := data.Labels().Get("containerIP")
						hostPort, err := strconv.Atoi(data.Labels().Get("hostPort"))
						assert.NilError(t, err)
						found, err := iptablesutil.ForwardExistsFromRules(rules, containerIP, hostPort)
						assert.NilError(t, err)
						assert.Assert(t, found)
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
					ExitCode: expect.ExitCodeGenericFail,
					Output: func(stdout string, t tig.T) {
						rules := strings.Split(stdout, "\n")
						containerIP := data.Labels().Get("containerIP")
						hostPort, err := strconv.Atoi(data.Labels().Get("hostPort"))
						assert.NilError(t, err)
						found, err := iptablesutil.ForwardExistsFromRules(rules, containerIP, hostPort)
						assert.NilError(t, err)
						assert.Assert(t, !found)
					},
				}
			},
		},
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())

		if p := data.Labels().Get("hostPort"); p != "" {
			if port, err := strconv.Atoi(p); err == nil {
				_ = portlock.Release(port)
			}
		}
	}

	testCase.Run(t)
}
