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

func TestVolumeInspectContainsLabels(t *testing.T) {
	const testVolume = "nerdctl-test-inspect-with-labels"

	base := testutil.NewBase(t)
	defer base.Cmd("volume", "rm", "-f", testVolume).Run()

	base.Cmd("volume", "create", "--label", "tag=testVolume", testVolume).AssertOK()
	inspect := base.InspectVolume(testVolume)
	inspectNerdctlLabels := (*inspect.Labels)
	expected := make(map[string]string, 1)
	expected["tag"] = "testVolume"
	assert.DeepEqual(base.T, expected, inspectNerdctlLabels)
}
