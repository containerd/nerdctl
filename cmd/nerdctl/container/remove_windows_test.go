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

package container

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestRemoveHyperVContainer(t *testing.T) {
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)

	if !testutil.HyperVSupported() {
		t.Skip("HyperV is not enabled, skipping test")
	}

	// ignore error
	base.Cmd("rm", tID, "-f").AssertOK()

	base.Cmd("run", "-d", "--isolation", "hyperv", "--name", tID, testutil.NginxAlpineImage).AssertOK()
	defer base.Cmd("rm", tID, "-f").AssertOK()

	base.EnsureContainerStarted(tID)
	inspect := base.InspectContainer(tID)
	//check with HCS if the container is ineed a VM
	isHypervContainer, err := testutil.HyperVContainer(inspect)
	if err != nil {
		t.Fatalf("unable to list HCS containers: %s", err)
	}

	assert.Assert(t, isHypervContainer, true)
	base.Cmd("rm", tID).AssertFail()

	base.Cmd("kill", tID).AssertOK()
	base.Cmd("rm", tID).AssertOK()
}
