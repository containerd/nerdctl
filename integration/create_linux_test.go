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

package integration

import (
	"fmt"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
	"github.com/containerd/nerdctl/pkg/testutil/nettestutil"
)

func TestCreate(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)

	base.Cmd("create", "--name", tID, testutil.CommonImage, "echo", "foo").AssertOK()
	defer base.Cmd("rm", "-f", tID).Run()
	base.Cmd("ps", "-a").AssertOutContains("Created")
	base.Cmd("start", tID).AssertOK()
	base.Cmd("logs", tID).AssertOutContains("foo")
}

func TestCreateWithMACAddress(t *testing.T) {
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)
	networkBridge := "testNetworkBridge" + tID
	networkMACvlan := "testNetworkMACvlan" + tID
	networkIPvlan := "testNetworkIPvlan" + tID
	base.Cmd("network", "create", networkBridge, "--driver", "bridge").AssertOK()
	base.Cmd("network", "create", networkMACvlan, "--driver", "macvlan").AssertOK()
	base.Cmd("network", "create", networkIPvlan, "--driver", "ipvlan").AssertOK()
	t.Cleanup(func() {
		base.Cmd("network", "rm", networkBridge).Run()
		base.Cmd("network", "rm", networkMACvlan).Run()
		base.Cmd("network", "rm", networkIPvlan).Run()
	})
	tests := []struct {
		Network string
		WantErr bool
		Expect  string
	}{
		{"host", true, "conflicting options"},
		{"none", true, "can't open '/sys/class/net/eth0/address'"},
		{"container:whatever" + tID, true, "conflicting options"},
		{"bridge", false, ""},
		{networkBridge, false, ""},
		{networkMACvlan, false, ""},
		{networkIPvlan, true, "not support"},
	}
	for i, test := range tests {
		containerName := fmt.Sprintf("%s_%d", tID, i)
		macAddress, err := nettestutil.GenerateMACAddress()
		if err != nil {
			t.Errorf("failed to generate MAC address: %s", err)
		}
		if test.Expect == "" && !test.WantErr {
			test.Expect = macAddress
		}
		t.Cleanup(func() {
			base.Cmd("rm", "-f", containerName).Run()
		})
		cmd := base.Cmd("create", "--network", test.Network, "--mac-address", macAddress, "--name", containerName, testutil.CommonImage, "cat", "/sys/class/net/eth0/address")
		if !test.WantErr {
			cmd.AssertOK()
			base.Cmd("start", containerName).AssertOK()
			cmd = base.Cmd("logs", containerName)
			cmd.AssertOK()
			cmd.AssertOutContains(test.Expect)
		} else {
			if (testutil.GetTarget() == testutil.Docker && test.Network == networkIPvlan) || test.Network == "none" {
				// 1. unlike nerdctl
				// when using network ipvlan in Docker
				// it delays fail on executing start command
				// 2. start on network none will success in both
				// nerdctl and Docker
				cmd.AssertOK()
				cmd = base.Cmd("start", containerName)
				if test.Network == "none" {
					// we check the result on logs command
					cmd.AssertOK()
					cmd = base.Cmd("logs", containerName)
				}
			}
			cmd.AssertCombinedOutContains(test.Expect)
			if test.Network == "none" {
				cmd.AssertOK()
			} else {
				cmd.AssertFail()
			}
		}
	}
}
