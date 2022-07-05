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
	"os"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
)

func TestImagePrune(t *testing.T) {
	testutil.RequiresBuild(t)

	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
	CMD ["echo", "nerdctl-test-image-prune"]`, testutil.CommonImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", imageName, buildCtx).AssertOK()
	base.Cmd("images").AssertOutContains(imageName)

	tID := testutil.Identifier(t)
	base.Cmd("run", "--name", tID, imageName).AssertOK()
	defer base.Cmd("rm", "-f", tID).Run()
	base.Cmd("image", "prune", "--force", "--all").AssertNoOut(imageName)
	base.Cmd("images").AssertOutContains(imageName)

	base.Cmd("rm", "-f", tID).AssertOK()
	base.Cmd("image", "prune", "--force", "--all").AssertOutContains(imageName)
	base.Cmd("images").AssertNoOut(imageName)
}
