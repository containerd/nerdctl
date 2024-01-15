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

// Test TestVolumeCreate for creating volume with given name.
func TestVolumeCreate(t *testing.T) {
	base := testutil.NewBase(t)
	testVolume := testutil.Identifier(t)

	base.Cmd("volume", "create", testVolume).AssertOK()
	defer base.Cmd("volume", "rm", "-f", testVolume).Run()

	base.Cmd("volume", "list").AssertOutContains(testVolume)
}

// Test TestVolumeCreateTooManyArgs for creating volume with too many args.
func TestVolumeCreateTooManyArgs(t *testing.T) {
	base := testutil.NewBase(t)

	base.Cmd("volume", "create", "too", "many").AssertFail()
}

// Test TestVolumeCreateWithLabels for creating volume with given labels.
func TestVolumeCreateWithLabels(t *testing.T) {
	base := testutil.NewBase(t)
	testVolume := testutil.Identifier(t)

	base.Cmd("volume", "create", testVolume, "--label", "foo1=baz1", "--label", "foo2=baz2").AssertOK()
	defer base.Cmd("volume", "rm", "-f", testVolume).Run()

	inspect := base.InspectVolume(testVolume)
	inspectNerdctlLabels := *inspect.Labels
	expected := make(map[string]string, 2)
	expected["foo1"] = "baz1"
	expected["foo2"] = "baz2"
	assert.DeepEqual(base.T, expected, inspectNerdctlLabels)
}
