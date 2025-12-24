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

package network

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestNetworkCreate(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.SubTests = []*test.Case{
		{
			Description: "vanilla",
			Setup: func(data test.Data, helpers test.Helpers) {
				identifier := data.Identifier()
				helpers.Ensure("network", "create", identifier)
				netw := nerdtest.InspectNetwork(helpers, identifier)
				assert.Equal(t, len(netw.IPAM.Config), 1)
				data.Labels().Set("subnet", netw.IPAM.Config[0].Subnet)

				helpers.Ensure("network", "create", data.Identifier("1"))
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("network", "rm", data.Identifier())
				helpers.Anyhow("network", "rm", data.Identifier("1"))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				data.Labels().Set("container2", helpers.Capture("run", "--rm", "--net", data.Identifier("1"), testutil.CommonImage, "ip", "route"))
				return helpers.Command("run", "--rm", "--net", data.Identifier(), testutil.CommonImage, "ip", "route")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Errors:   nil,
					Output: func(stdout string, t tig.T) {
						assert.Assert(t, strings.Contains(stdout, data.Labels().Get("subnet")))
						assert.Assert(t, !strings.Contains(data.Labels().Get("container2"), data.Labels().Get("subnet")))
					},
				}
			},
		},
		{
			Description: "with MTU",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("network", "create", data.Identifier(), "--driver", "bridge", "--opt", "com.docker.network.driver.mtu=9216")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("network", "rm", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--net", data.Identifier(), testutil.CommonImage, "ifconfig", "eth0")
			},
			Expected: test.Expects(0, nil, expect.Contains("MTU:9216")),
		},
		{
			Description: "with ipv6",
			Require:     nerdtest.OnlyIPv6,
			Setup: func(data test.Data, helpers test.Helpers) {
				subnetStr := "2001:db8:8::/64"
				data.Labels().Set("subnetStr", subnetStr)
				_, _, err := net.ParseCIDR(subnetStr)
				assert.Assert(t, err == nil)

				helpers.Ensure("network", "create", data.Identifier(), "--ipv6", "--subnet", subnetStr)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("network", "rm", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--net", data.Identifier(), testutil.CommonImage, "ip", "addr", "show", "dev", "eth0")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, t tig.T) {
						_, subnet, _ := net.ParseCIDR(data.Labels().Get("subnetStr"))
						ip := nerdtest.FindIPv6(stdout)
						assert.Assert(t, subnet.Contains(ip), fmt.Sprintf("subnet %s contains ip %s", subnet, ip))
					},
				}
			},
		},
		{
			Description: "internal enabled",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("network", "create", "--internal", data.Identifier())
				netw := nerdtest.InspectNetwork(helpers, data.Identifier())
				assert.Equal(t, len(netw.IPAM.Config), 1)
				data.Labels().Set("subnet", netw.IPAM.Config[0].Subnet)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("network", "rm", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--net", data.Identifier(), testutil.CommonImage, "ip", "route")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, t tig.T) {
						assert.Assert(t, strings.Contains(stdout, data.Labels().Get("subnet")))
						assert.Assert(t, !strings.Contains(stdout, "default "))
						if nerdtest.IsDocker() {
							return
						}
						nativeNet := nerdtest.InspectNetworkNative(helpers, data.Identifier())
						var cni struct {
							Plugins []struct {
								Type   string `json:"type"`
								IsGW   bool   `json:"isGateway"`
								IPMasq bool   `json:"ipMasq"`
							} `json:"plugins"`
						}
						_ = json.Unmarshal(nativeNet.CNI, &cni)
						// bridge plugin assertions and no portmap
						foundBridge := false
						for _, p := range cni.Plugins {
							assert.Assert(t, p.Type != "portmap")
							if p.Type == "bridge" {
								foundBridge = true
								assert.Assert(t, !p.IsGW)
								assert.Assert(t, !p.IPMasq)
							}
						}
						assert.Assert(t, foundBridge)
					},
				}
			},
		},
		{
			Description: "with static IPv4 address",
			Setup: func(data test.Data, helpers test.Helpers) {
				networkName := data.Identifier()
				staticIP := "172.19.0.100"
				data.Labels().Set("networkName", networkName)
				data.Labels().Set("staticIP", staticIP)
				helpers.Ensure("network", "create", networkName, "--driver", "bridge", "--subnet", "172.19.0.0/24")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("network", "rm", data.Labels().Get("networkName"))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--net", data.Labels().Get("networkName"), "--ip", data.Labels().Get("staticIP"), testutil.CommonImage, "ip", "addr", "show", "eth0")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, t tig.T) {
						assert.Assert(t, strings.Contains(stdout, fmt.Sprintf("inet %s/24", data.Labels().Get("staticIP"))))
					},
				}
			},
		},
		{
			Description: "with static IPv6 address",
			Require:     nerdtest.OnlyIPv6,
			Setup: func(data test.Data, helpers test.Helpers) {
				networkName := data.Identifier()
				staticIPv6 := "2001:db8:1::100"
				data.Labels().Set("networkName", networkName)
				data.Labels().Set("staticIPv6", staticIPv6)
				helpers.Ensure("network", "create", networkName, "--driver", "bridge", "--ipv6", "--subnet", "2001:db8:1::/64")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("network", "rm", data.Labels().Get("networkName"))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--net", data.Labels().Get("networkName"), "--ip", data.Labels().Get("staticIPv6"), testutil.CommonImage, "ip", "addr", "show", "eth0")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, t tig.T) {
						assert.Assert(t, strings.Contains(stdout, fmt.Sprintf("inet6 %s/64", data.Labels().Get("staticIPv6"))))
					},
				}
			},
		},
		{
			Description: "with dual-stack static IP addresses",
			Require:     nerdtest.OnlyIPv6,
			Setup: func(data test.Data, helpers test.Helpers) {
				networkName := data.Identifier()
				staticIPv4 := "172.20.0.100"
				staticIPv6 := "2001:db8:2::100"
				data.Labels().Set("networkName", networkName)
				data.Labels().Set("staticIPv4", staticIPv4)
				data.Labels().Set("staticIPv6", staticIPv6)
				helpers.Ensure("network", "create", networkName, "--driver", "bridge", "--subnet", "172.20.0.0/24", "--ipv6", "--subnet", "2001:db8:2::/64")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("network", "rm", data.Labels().Get("networkName"))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--net", data.Labels().Get("networkName"), "--ip", data.Labels().Get("staticIPv4"), "--ip", data.Labels().Get("staticIPv6"), testutil.CommonImage, "ip", "addr", "show", "eth0")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, t tig.T) {
						assert.Assert(t, strings.Contains(stdout, fmt.Sprintf("inet %s/24", data.Labels().Get("staticIPv4"))))
						assert.Assert(t, strings.Contains(stdout, fmt.Sprintf("inet6 %s/64", data.Labels().Get("staticIPv6"))))
					},
				}
			},
		},
		{
			Description: "with static IPv6 address on macvlan",
			Require:     nerdtest.OnlyIPv6,
			Setup: func(data test.Data, helpers test.Helpers) {
				dummyLinkName := "dummy-" + data.Identifier()
				networkName := data.Identifier()
				staticIPv6 := "2001:db8:3::100"
				subnet := "2001:db8:3::/64"

				data.Labels().Set("dummyLinkName", dummyLinkName)
				data.Labels().Set("networkName", networkName)
				data.Labels().Set("staticIPv6", staticIPv6)

				// Create a dummy interface to be the parent of the macvlan network
				helpers.Custom("ip", "link", "add", dummyLinkName, "type", "dummy").Run(&test.Expected{ExitCode: 0})
				helpers.Custom("ip", "link", "set", dummyLinkName, "up").Run(&test.Expected{ExitCode: 0})

				// Create the macvlan network
				helpers.Ensure("network", "create", networkName,
					"--driver", "macvlan",
					"--parent", dummyLinkName,
					"--ipv6",
					"--subnet", subnet)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("network", "rm", data.Labels().Get("networkName"))
				helpers.Anyhow("ip", "link", "del", data.Labels().Get("dummyLinkName"))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm",
					"--net", data.Labels().Get("networkName"),
					"--ip6", data.Labels().Get("staticIPv6"),
					testutil.CommonImage, "ip", "addr", "show", "eth0")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, t tig.T) {
						assert.Assert(t, strings.Contains(stdout, fmt.Sprintf("inet6 %s/64", data.Labels().Get("staticIPv6"))))
					},
				}
			},
		},
	}

	testCase.Run(t)
}

func TestNetworkCreateICC(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = require.All(
		require.Linux,
	)

	testCase.SubTests = []*test.Case{
		{
			Description: "with enable_icc=false",
			Require:     nerdtest.CNIFirewallVersion("1.7.1"),
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				// Create a network with ICC disabled
				helpers.Ensure("network", "create", data.Identifier(), "--driver", "bridge",
					"--opt", "com.docker.network.bridge.enable_icc=false")

				// Run a container in that network
				data.Labels().Set("container1", helpers.Capture("run", "-d", "--net", data.Identifier(),
					"--name", data.Identifier("c1"), testutil.CommonImage, "sleep", "infinity"))

				// Wait for container to be running
				nerdtest.EnsureContainerStarted(helpers, data.Identifier("c1"))
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("container", "rm", "-f", data.Identifier("c1"))
				helpers.Anyhow("network", "rm", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				// DEBUG: Check br_netfilter module status
				helpers.Custom("sh", "-ec", "lsmod | grep br_netfilter || echo 'br_netfilter not loaded'").Run(&test.Expected{})
				helpers.Custom("sh", "-ec", "cat /proc/sys/net/bridge/bridge-nf-call-iptables 2>/dev/null || echo 'bridge-nf-call-iptables not available'").Run(&test.Expected{})
				helpers.Custom("sh", "-ec", "ls /proc/sys/net/bridge/ 2>/dev/null || echo 'bridge sysctl not available'").Run(&test.Expected{})
				// Try to ping the other container in the same network
				// This should fail when ICC is disabled
				return helpers.Command("run", "--rm", "--net", data.Identifier(),
					testutil.CommonImage, "ping", "-c", "1", "-W", "1", data.Identifier("c1"))
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil), // Expect ping to fail with exit code 1
		},
		{
			Description: "with enable_icc=true",
			Require:     nerdtest.CNIFirewallVersion("1.7.1"),
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				// Create a network with ICC enabled (default)
				helpers.Ensure("network", "create", data.Identifier(), "--driver", "bridge",
					"--opt", "com.docker.network.bridge.enable_icc=true")

				// Run a container in that network
				data.Labels().Set("container1", helpers.Capture("run", "-d", "--net", data.Identifier(),
					"--name", data.Identifier("c1"), testutil.CommonImage, "sleep", "infinity"))
				// Wait for container to be running
				nerdtest.EnsureContainerStarted(helpers, data.Identifier("c1"))
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("container", "rm", "-f", data.Identifier("c1"))
				helpers.Anyhow("network", "rm", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				// Try to ping the other container in the same network
				// This should succeed when ICC is enabled
				return helpers.Command("run", "--rm", "--net", data.Identifier(),
					testutil.CommonImage, "ping", "-c", "1", "-W", "1", data.Identifier("c1"))
			},
			Expected: test.Expects(0, nil, nil), // Expect ping to succeed with exit code 0
		},
		{
			Description: "with no enable_icc option set",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				// Create a network with ICC enabled (default)
				helpers.Ensure("network", "create", data.Identifier(), "--driver", "bridge")

				// Run a container in that network
				data.Labels().Set("container1", helpers.Capture("run", "-d", "--net", data.Identifier(),
					"--name", data.Identifier("c1"), testutil.CommonImage, "sleep", "infinity"))
				// Wait for container to be running
				nerdtest.EnsureContainerStarted(helpers, data.Identifier("c1"))
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("container", "rm", "-f", data.Identifier("c1"))
				helpers.Anyhow("network", "rm", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				// Try to ping the other container in the same network
				// This should succeed when no ICC is set
				return helpers.Command("run", "--rm", "--net", data.Identifier(),
					testutil.CommonImage, "ping", "-c", "1", "-W", "1", data.Identifier("c1"))
			},
			Expected: test.Expects(0, nil, nil), // Expect ping to succeed with exit code 0
		},
	}

	testCase.Run(t)
}
