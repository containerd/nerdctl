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

package image

import (
	"fmt"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	testhelpers "github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
	"github.com/containerd/nerdctl/v2/pkg/testutil/testregistry"
)

func TestImageEncryptJWE(t *testing.T) {
	nerdtest.Setup()

	var registry *testregistry.RegistryServer
	var keyPair *testhelpers.JweKeyPair

	const remoteImageKey = "remoteImageKey"

	testCase := &test.Case{
		Require: test.Require(
			test.Linux,
			test.Not(nerdtest.Docker),
			// This test needs to rmi the common image
			nerdtest.Private,
		),
		Cleanup: func(data test.Data, helpers test.Helpers) {
			if registry != nil {
				registry.Cleanup(nil)
				keyPair.Cleanup()
				helpers.Anyhow("rmi", "-f", data.Get(remoteImageKey))
			}
			helpers.Anyhow("rmi", "-f", data.Identifier("decrypted"))
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			base := testutil.NewBase(t)
			registry = testregistry.NewWithNoAuth(base, 0, false)
			keyPair = testhelpers.NewJWEKeyPair(t)
			helpers.Ensure("pull", "--quiet", testutil.CommonImage)
			encryptImageRef := fmt.Sprintf("127.0.0.1:%d/%s:encrypted", registry.Port, data.Identifier())
			helpers.Ensure("image", "encrypt", "--recipient=jwe:"+keyPair.Pub, testutil.CommonImage, encryptImageRef)
			inspector := helpers.Capture("image", "inspect", "--mode=native", "--format={{len .Index.Manifests}}", encryptImageRef)
			assert.Equal(t, inspector, "1\n")
			inspector = helpers.Capture("image", "inspect", "--mode=native", "--format={{json .Manifest.Layers}}", encryptImageRef)
			assert.Assert(t, strings.Contains(inspector, "org.opencontainers.image.enc.keys.jwe"))
			helpers.Ensure("push", encryptImageRef)
			helpers.Anyhow("rmi", "-f", encryptImageRef)
			helpers.Anyhow("rmi", "-f", testutil.CommonImage)
			data.Set(remoteImageKey, encryptImageRef)
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			helpers.Fail("pull", data.Get(remoteImageKey))
			helpers.Ensure("pull", "--quiet", "--unpack=false", data.Get(remoteImageKey))
			helpers.Fail("image", "decrypt", "--key="+keyPair.Pub, data.Get(remoteImageKey), data.Identifier("decrypted")) // decryption needs prv key, not pub key
			return helpers.Command("image", "decrypt", "--key="+keyPair.Prv, data.Get(remoteImageKey), data.Identifier("decrypted"))
		},
		Expected: test.Expects(0, nil, nil),
	}

	testCase.Run(t)
}
