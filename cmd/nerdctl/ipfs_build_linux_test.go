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
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestIPFSBuild(t *testing.T) {
	testutil.DockerIncompatible(t)
	testutil.RequiresBuild(t)
	testutil.RegisterBuildCacheCleanup(t)
	base := testutil.NewBase(t)
	ipfsCID := pushImageToIPFS(t, base, testutil.AlpineImage)
	ipfsCIDBase := strings.TrimPrefix(ipfsCID, "ipfs://")

	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM localhost:5050/ipfs/%s
CMD ["echo", "nerdctl-build-test-string"]
	`, ipfsCIDBase)

	buildCtx := helpers.CreateBuildContext(t, dockerfile)

	done := ipfsRegistryUp(t, base)
	defer done()
	base.Cmd("build", "-t", imageName, buildCtx).AssertOK()
	base.Cmd("build", buildCtx, "-t", imageName).AssertOK()

	base.Cmd("run", "--rm", imageName).AssertOutContains("nerdctl-build-test-string")
}

func ipfsRegistryUp(t *testing.T, base *testutil.Base, args ...string) (done func() error) {
	res := icmd.StartCmd(base.Cmd(append([]string{"ipfs", "registry", "serve"}, args...)...).Cmd)
	time.Sleep(time.Second)
	assert.Assert(t, res.Cmd.Process != nil)
	assert.NilError(t, res.Error)
	return func() error {
		res.Cmd.Process.Kill()
		icmd.WaitOnCmd(3*time.Second, res)
		return nil
	}
}
