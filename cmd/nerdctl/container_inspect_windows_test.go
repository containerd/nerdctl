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

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"gotest.tools/v3/assert"
)

func TestInspectProcessContainerContainsLabel(t *testing.T) {
	testContainer := testutil.Identifier(t)

	base := testutil.NewBase(t)
	defer base.Cmd("rm", "-f", testContainer).Run()

	base.Cmd("run", "-d", "--name", testContainer, "--label", "foo=foo", "--label", "bar=bar", testutil.NginxAlpineImage).AssertOK()
	base.EnsureContainerStarted(testContainer)
	inspect := base.InspectContainer(testContainer)
	lbs := inspect.Config.Labels

	assert.Equal(base.T, "foo", lbs["foo"])
	assert.Equal(base.T, "bar", lbs["bar"])
}

func TestInspectHyperVContainerContainsLabel(t *testing.T) {
	if !testutil.HyperVSupported() {
		t.Skip("HyperV is not enabled, skipping test")
	}

	testContainer := testutil.Identifier(t)

	base := testutil.NewBase(t)
	defer base.Cmd("rm", "-f", testContainer).Run()

	base.Cmd("run", "-d", "--name", testContainer, "--isolation", "hyperv", "--label", "foo=foo", "--label", "bar=bar", testutil.NginxAlpineImage).AssertOK()
	base.EnsureContainerStarted(testContainer)
	inspect := base.InspectContainer(testContainer)
	lbs := inspect.Config.Labels

	//check with HCS if the container is ineed a VM
	isHypervContainer, err := testutil.HyperVContainer(inspect)
	if err != nil {
		t.Fatalf("unable to list HCS containers: %s", err)
	}

	assert.Assert(t, isHypervContainer, true)
	assert.Equal(base.T, "foo", lbs["foo"])
	assert.Equal(base.T, "bar", lbs["bar"])
}
