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
	"os/exec"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
	"github.com/containerd/nerdctl/pkg/testutil/testregistry"
	"gotest.tools/v3/assert"
)

func TestRunVerifyCosign(t *testing.T) {
	if _, err := exec.LookPath("cosign"); err != nil {
		t.Skip()
	}
	testutil.DockerIncompatible(t)
	testutil.RequiresBuild(t)
	t.Setenv("COSIGN_PASSWORD", "1")
	keyPair := newCosignKeyPair(t, "cosign-key-pair")
	defer keyPair.cleanup()
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	tID := testutil.Identifier(t)
	reg := testregistry.NewPlainHTTP(base, 5000)
	defer reg.Cleanup()
	localhostIP := "127.0.0.1"
	t.Logf("localhost IP=%q", localhostIP)
	testImageRef := fmt.Sprintf("%s:%d/%s",
		localhostIP, reg.ListenPort, tID)
	t.Logf("testImageRef=%q", testImageRef)

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", testImageRef, buildCtx).AssertOK()
	base.Cmd("push", testImageRef, "--sign=cosign", "--cosign-key="+keyPair.privateKey).AssertOK()
	base.Cmd("run", "--rm", "--verify=cosign", "--cosign-key="+keyPair.publicKey, testImageRef).AssertOK()
	base.Cmd("run", "--rm", "--verify=cosign", "--cosign-key=dummy", testImageRef).AssertFail()
}
