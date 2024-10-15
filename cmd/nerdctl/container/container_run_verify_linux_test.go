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
	"fmt"
	"testing"

	testhelpers "github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/registry"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestRunVerifyCosign(t *testing.T) {
	var keyPair *testhelpers.CosignKeyPair
	var reg *registry.Server

	testCase := nerdtest.Setup()

	testCase.Require = test.Require(
		test.Binary("cosign"),
		test.Not(nerdtest.Docker),
		nerdtest.Build,
	)

	testCase.Env = map[string]string{
		"COSIGN_PASSWORD": "1",
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		keyPair = testhelpers.NewCosignKeyPair(t, "cosign-key-pair", "1")
		reg = nerdtest.RegistryWithNoAuth(data, helpers, 0, false)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if keyPair != nil {
			keyPair.Cleanup()
		}
		if reg != nil {
			reg.Cleanup(data, helpers)
		}
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		testImageRef := fmt.Sprintf("127.0.0.1:%d/%s", reg.Port, data.Identifier())
		dockerfile := fmt.Sprintf(`FROM %s
		CMD ["echo", "nerdctl-build-test-string"]
			`, testutil.CommonImage)

		buildCtx := testhelpers.CreateBuildContext(t, dockerfile)

		helpers.Ensure("build", "-t", testImageRef, buildCtx)
		helpers.Ensure("push", testImageRef, "--sign=cosign", "--cosign-key="+keyPair.PrivateKey)
		helpers.Ensure("run", "--rm", "--verify=cosign", "--cosign-key="+keyPair.PublicKey, testImageRef)
		return helpers.Command("run", "--rm", "--verify=cosign", "--cosign-key=dummy", testImageRef)
	}

	testCase.Expected = test.Expects(1, nil, nil)
}
