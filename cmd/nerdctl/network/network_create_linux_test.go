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
	"net"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	ipv6helper "github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestNetworkCreate(t *testing.T) {
	nerdtest.Setup()

	testGroup := &test.Group{
		{
			Description: "Network create",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("network", "create", data.Identifier())
				netw := nerdtest.InspectNetwork(helpers, data.Identifier())
				assert.Equal(t, len(netw.IPAM.Config), 1)
				data.Set("subnet", netw.IPAM.Config[0].Subnet)

				helpers.Ensure("network", "create", data.Identifier()+"-1")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("network", "rm", data.Identifier())
				helpers.Anyhow("network", "rm", data.Identifier()+"-1")
			},
			Command: func(data test.Data, helpers test.Helpers) test.Command {
				data.Set("container2", helpers.Capture("run", "--rm", "--net", data.Identifier()+"-1", testutil.AlpineImage, "ip", "route"))
				return helpers.Command("run", "--rm", "--net", data.Identifier(), testutil.AlpineImage, "ip", "route")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Errors:   nil,
					Output: func(stdout string, info string, t *testing.T) {
						assert.Assert(t, strings.Contains(stdout, data.Get("subnet")), info)
						assert.Assert(t, !strings.Contains(data.Get("container2"), data.Get("subnet")), info)
					},
				}
			},
		},
		{
			Description: "Network create with MTU",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("network", "create", data.Identifier(), "--driver", "bridge", "--opt", "com.docker.network.driver.mtu=9216")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("network", "rm", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.Command {
				return helpers.Command("run", "--rm", "--net", data.Identifier(), testutil.AlpineImage, "ifconfig", "eth0")
			},
			Expected: test.Expects(0, nil, test.Contains("MTU:9216")),
		},
		{
			Description: "Network create with ipv6",
			Require:     nerdtest.OnlyIPv6,
			Setup: func(data test.Data, helpers test.Helpers) {
				subnetStr := "2001:db8:8::/64"
				data.Set("subnetStr", subnetStr)
				_, _, err := net.ParseCIDR(subnetStr)
				assert.Assert(t, err == nil)

				helpers.Ensure("network", "create", data.Identifier(), "--ipv6", "--subnet", subnetStr)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("network", "rm", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.Command {
				return helpers.Command("run", "--rm", "--net", data.Identifier(), testutil.CommonImage, "ip", "addr", "show", "dev", "eth0")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, info string, t *testing.T) {
						_, subnet, _ := net.ParseCIDR(data.Get("subnetStr"))
						ip := ipv6helper.FindIPv6(stdout)
						assert.Assert(t, subnet.Contains(ip), info)
					},
				}
			},
		},
	}

	testGroup.Run(t)
}
