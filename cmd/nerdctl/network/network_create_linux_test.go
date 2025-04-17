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
	"fmt"
	"net"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

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
					Output: func(stdout string, info string, t *testing.T) {
						assert.Assert(t, strings.Contains(stdout, data.Labels().Get("subnet")), info)
						assert.Assert(t, !strings.Contains(data.Labels().Get("container2"), data.Labels().Get("subnet")), info)
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
					Output: func(stdout string, info string, t *testing.T) {
						_, subnet, _ := net.ParseCIDR(data.Labels().Get("subnetStr"))
						ip := nerdtest.FindIPv6(stdout)
						assert.Assert(t, subnet.Contains(ip), fmt.Sprintf("subnet %s contains ip %s", subnet, ip))
					},
				}
			},
		},
	}

	testCase.Run(t)
}
