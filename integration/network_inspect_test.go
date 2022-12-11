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
	"runtime"
	"testing"

	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
)

func TestNetworkInspect(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("IPAMConfig not implemented on Windows yet")
	}

	testNetwork := testutil.Identifier(t)
	const (
		testSubnet  = "10.24.24.0/24"
		testGateway = "10.24.24.1"
		testIPRange = "10.24.24.1/25"
	)

	base := testutil.NewBase(t)
	defer base.Cmd("network", "rm", testNetwork).Run()

	args := []string{
		"network", "create", "--label", "tag=testNetwork", "--subnet", testSubnet,
		"--gateway", testGateway, "--ip-range", testIPRange,
		testNetwork,
	}
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
				IPRange: testIPRange,
			},
		},
	}
	assert.DeepEqual(base.T, expectedIPAM, got.IPAM)
}
