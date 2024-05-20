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
	"fmt"
	"runtime"
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
	"gotest.tools/v3/assert"
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

func TestCreateWithLabel(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)

	base.Cmd("create", "--name", tID, "--label", "foo=bar", testutil.NginxAlpineImage, "echo", "foo").AssertOK()
	defer base.Cmd("rm", "-f", tID).Run()
	inspect := base.InspectContainer(tID)
	assert.Equal(base.T, "bar", inspect.Config.Labels["foo"])
	// the label `maintainer`` is defined by image
	assert.Equal(base.T, "NGINX Docker Maintainers <docker-maint@nginx.com>", inspect.Config.Labels["maintainer"])
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
		testName := fmt.Sprintf("%s_container:%s_network:%s_expect:%s", tID, containerName, test.Network, test.Expect)
		t.Run(testName, func(tt *testing.T) {
			macAddress, err := nettestutil.GenerateMACAddress()
			if err != nil {
				tt.Errorf("failed to generate MAC address: %s", err)
			}
			if test.Expect == "" && !test.WantErr {
				test.Expect = macAddress
			}
			tt.Cleanup(func() {
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
		})
	}
}

func TestCreateWithTty(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("json-file log driver is not yet implemented on Windows")
	}
	base := testutil.NewBase(t)
	imageName := testutil.CommonImage
	withoutTtyContainerName := "without-terminal-" + testutil.Identifier(t)
	withTtyContainerName := "with-terminal-" + testutil.Identifier(t)

	// without -t, fail
	base.Cmd("create", "--name", withoutTtyContainerName, imageName, "stty").AssertOK()
	base.Cmd("start", withoutTtyContainerName).AssertOK()
	defer base.Cmd("container", "rm", "-f", withoutTtyContainerName).AssertOK()
	base.Cmd("logs", withoutTtyContainerName).AssertCombinedOutContains("stty: standard input: Not a tty")
	withoutTtyContainer := base.InspectContainer(withoutTtyContainerName)
	assert.Equal(base.T, 1, withoutTtyContainer.State.ExitCode)

	// with -t, success
	base.Cmd("create", "-t", "--name", withTtyContainerName, imageName, "stty").AssertOK()
	base.Cmd("start", withTtyContainerName).AssertOK()
	defer base.Cmd("container", "rm", "-f", withTtyContainerName).AssertOK()
	base.Cmd("logs", withTtyContainerName).AssertCombinedOutContains("speed 38400 baud; line = 0;")
	withTtyContainer := base.InspectContainer(withTtyContainerName)
	assert.Equal(base.T, 0, withTtyContainer.State.ExitCode)
}
