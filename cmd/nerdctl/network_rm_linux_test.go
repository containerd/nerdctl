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

package main

import (
	"testing"

	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/containerd/nerdctl/pkg/testutil"
	"github.com/vishvananda/netlink"
	"gotest.tools/v3/assert"
)

func TestNetworkRemove(t *testing.T) {
	t.Parallel()
	if rootlessutil.IsRootless() {
		t.Skip("test skipped for remove rootless network")
	}
	base := testutil.NewBase(t)
	networkName := testutil.Identifier(t)

	base.Cmd("network", "create", networkName).AssertOK()
	defer base.Cmd("network", "rm", networkName).Run()

	networkID := base.InspectNetwork(networkName).ID

	tID := testutil.Identifier(t)
	base.Cmd("run", "--rm", "--net", networkName, "--name", tID, testutil.CommonImage).AssertOK()

	_, err := netlink.LinkByName("br-" + networkID[:12])
	assert.NilError(t, err)

	base.Cmd("network", "rm", networkName).AssertOK()

	_, err = netlink.LinkByName("br-" + networkID[:12])
	assert.Error(t, err, "Link not found")
}
