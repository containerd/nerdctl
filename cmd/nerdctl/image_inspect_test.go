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

func TestImageInspectContainsSomeStuff(t *testing.T) {
	base := testutil.NewBase(t)
	defer base.Cmd("rmi", "-f", testutil.NginxAlpineImage).Run()

	base.Cmd("pull", testutil.NginxAlpineImage).AssertOK()
	inspect := base.InspectImage(testutil.NginxAlpineImage)

	assert.Assert(base.T, len(inspect.RootFS.Layers) > 0)
	assert.Assert(base.T, inspect.RootFS.Type != "")
	assert.Assert(base.T, inspect.Architecture != "")
}

func TestImageInspectWithImageID(t *testing.T) {
	base := testutil.NewBase(t)
	defer base.Cmd("rmi", "-f", testutil.NginxAlpineImage).Run()

	base.Cmd("pull", testutil.NginxAlpineImage).AssertOK()
	//fetch the Image ID
	inspect := base.InspectImage(testutil.NginxAlpineImage)

	//Make an inspect with the Image ID
	inspectWithImageId := base.InspectImage(inspect.ID)

	assert.Assert(base.T, len(inspectWithImageId.RootFS.Layers) > 0)
	assert.Assert(base.T, inspectWithImageId.RootFS.Type != "")
	assert.Assert(base.T, inspectWithImageId.Architecture != "")
}
