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
	"errors"
	"testing"

	"github.com/vishvananda/netlink"
	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestNetworkRemove(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = nerdtest.RootFul

	testCase.SubTests = []*test.Case{
		{
			Description: "Simple network remove",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("network", "create", data.Identifier())
				data.Set("netID", nerdtest.InspectNetwork(helpers, data.Identifier()).ID)
				helpers.Ensure("run", "--rm", "--net", data.Identifier(), "--name", data.Identifier(), testutil.CommonImage)
				// Verity the network is here
				_, err := netlink.LinkByName("br-" + data.Get("netID")[:12])
				assert.NilError(t, err, "failed to find network br-"+data.Get("netID")[:12], "%v")
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("network", "rm", data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("network", "rm", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, info string, t *testing.T) {
						_, err := netlink.LinkByName("br-" + data.Get("netID")[:12])
						assert.Error(t, err, "Link not found", info)
					},
				}
			},
		},
		{
			Description: "Network remove when linked to container",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("network", "create", data.Identifier())
				helpers.Ensure("run", "-d", "--net", data.Identifier(), "--name", data.Identifier(), testutil.CommonImage, "sleep", "infinity")
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("network", "rm", data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
				helpers.Anyhow("network", "rm", data.Identifier())
			},
			Expected: test.Expects(1, []error{errors.New("is in use")}, nil),
		},
		{
			Description: "Network remove by id",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("network", "create", data.Identifier())
				data.Set("netID", nerdtest.InspectNetwork(helpers, data.Identifier()).ID)
				helpers.Ensure("run", "--rm", "--net", data.Identifier(), "--name", data.Identifier(), testutil.CommonImage)
				// Verity the network is here
				_, err := netlink.LinkByName("br-" + data.Get("netID")[:12])
				assert.NilError(t, err, "failed to find network br-"+data.Get("netID")[:12], "%v")
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("network", "rm", data.Get("netID"))
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("network", "rm", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, info string, t *testing.T) {
						_, err := netlink.LinkByName("br-" + data.Get("netID")[:12])
						assert.Error(t, err, "Link not found", info)
					},
				}
			},
		},
		{
			Description: "Network remove by short id",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("network", "create", data.Identifier())
				data.Set("netID", nerdtest.InspectNetwork(helpers, data.Identifier()).ID)
				helpers.Ensure("run", "--rm", "--net", data.Identifier(), "--name", data.Identifier(), testutil.CommonImage)
				// Verity the network is here
				_, err := netlink.LinkByName("br-" + data.Get("netID")[:12])
				assert.NilError(t, err, "failed to find network br-"+data.Get("netID")[:12], "%v")
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("network", "rm", data.Get("netID")[:12])
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("network", "rm", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, info string, t *testing.T) {
						_, err := netlink.LinkByName("br-" + data.Get("netID")[:12])
						assert.Error(t, err, "Link not found", info)
					},
				}
			},
		},
	}

	testCase.Run(t)
}
