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

	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
)

func TestNetworkCreateWithMTU(t *testing.T) {
	testNetwork := testutil.Identifier(t)
	base := testutil.NewBase(t)

	args := []string{
		"network", "create", testNetwork,
		"--driver", "bridge", "--opt", "com.docker.network.driver.mtu=9216",
	}
	base.Cmd(args...).AssertOK()
	defer base.Cmd("network", "rm", testNetwork).AssertOK()

	base.Cmd("run", "--rm", "--net", testNetwork, testutil.AlpineImage, "ifconfig", "eth0").AssertOutContains("MTU:9216")
}

func TestNetworkCreate(t *testing.T) {
	base := testutil.NewBase(t)
	testNetwork := testutil.Identifier(t)

	base.Cmd("network", "create", testNetwork).AssertOK()
	defer base.Cmd("network", "rm", testNetwork).AssertOK()

	net := base.InspectNetwork(testNetwork)
	assert.Equal(t, len(net.IPAM.Config), 1)

	base.Cmd("run", "--rm", "--net", testNetwork, testutil.CommonImage, "ip", "route").AssertOutContains(net.IPAM.Config[0].Subnet)

	base.Cmd("network", "create", testNetwork+"-1").AssertOK()
	defer base.Cmd("network", "rm", testNetwork+"-1").AssertOK()

	base.Cmd("run", "--rm", "--net", testNetwork+"-1", testutil.CommonImage, "ip", "route").AssertNoOut(net.IPAM.Config[0].Subnet)
}
