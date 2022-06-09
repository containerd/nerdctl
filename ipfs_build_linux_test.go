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
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"

	"gotest.tools/v3/assert"
)

func TestIPFSBuild(t *testing.T) {
	testutil.DockerIncompatible(t)
	requiresIPFS(t)
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	ipfsCID := pushImageToIPFS(t, base, testutil.AlpineImage)
	ipfsCIDBase := strings.TrimPrefix(ipfsCID, "ipfs://")

	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM localhost:5050/ipfs/%s
CMD ["echo", "nerdctl-build-test-string"]
	`, ipfsCIDBase)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	defer base.Cmd("ipfs", "registry", "down").AssertOK()
	base.Cmd("build", "--ipfs", "-t", imageName, buildCtx).AssertOK()
	base.Cmd("build", buildCtx, "--ipfs", "-t", imageName).AssertOK()

	base.Cmd("run", "--rm", imageName).AssertOutContains("nerdctl-build-test-string")
}
