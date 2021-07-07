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

	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
)

func TestNetworkInspect(t *testing.T) {
	const (
		testNetwork = "nerdctl-test-network-inspect"
		testSubnet  = "10.24.24.0/24"
		testGateway = "10.24.24.1"
	)

	base := testutil.NewBase(t)
	defer base.Cmd("network", "rm", testNetwork).Run()

	args := []string{
		"network", "create", "--label", "tag=testNetwork", "--subnet", testSubnet,
	}
	if base.Target == testutil.Docker {
		// trivial incompatibility: nerdctl computes gateway automatically, but docker does not
		args = append(args, "--gateway", testGateway)
	}
	args = append(args, testNetwork)
	base.Cmd(args...).AssertOK()
	got := base.InspectNetwork(testNetwork)

	assert.DeepEqual(base.T, testNetwork, got.Name)

	expectedLabels := map[string]string{
		"tag": "testNetwork",
	}
	assert.DeepEqual(base.T, expectedLabels, got.Labels)

	expectedIPAM := dockercompat.IPAM{
		Config: []dockercompat.IPAMConfig{
			{
				Subnet:  testSubnet,
				Gateway: testGateway,
			},
		},
	}
	assert.DeepEqual(base.T, expectedIPAM, got.IPAM)
}
