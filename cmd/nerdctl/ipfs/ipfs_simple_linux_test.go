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

package ipfs

import (
	"regexp"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestIPFSSimple(t *testing.T) {
	testCase := nerdtest.Setup()

	const mainImageCIDKey = "mainImageCIDKey"
	const transformedImageCIDKey = "transformedImageCIDKey"

	testCase.Require = require.All(
		require.Linux,
		require.Not(nerdtest.Docker),
		nerdtest.IPFS,
		// We constantly rmi the image by its CID which is shared across tests, so, we make this group private
		// and every subtest NoParallel
		nerdtest.Private,
	)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("pull", "--quiet", testutil.CommonImage)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "with default snapshotter",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				data.Labels().Set(mainImageCIDKey, pushToIPFS(helpers, testutil.CommonImage))
				helpers.Ensure("pull", "--quiet", "ipfs://"+data.Labels().Get(mainImageCIDKey))
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				if data.Labels().Get(mainImageCIDKey) != "" {
					helpers.Anyhow("rmi", "-f", data.Labels().Get(mainImageCIDKey))
				}
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", data.Labels().Get(mainImageCIDKey), "echo", "hello")
			},
			Expected: test.Expects(0, nil, expect.Equals("hello\n")),
		},
		{
			Description: "with stargz snapshotter",
			NoParallel:  true,
			Require: require.All(
				nerdtest.Stargz,
				nerdtest.NerdctlNeedsFixing("https://github.com/containerd/nerdctl/issues/3475"),
			),
			Setup: func(data test.Data, helpers test.Helpers) {
				data.Labels().Set(mainImageCIDKey, pushToIPFS(helpers, testutil.CommonImage, "--estargz"))
				helpers.Ensure("pull", "--quiet", "ipfs://"+data.Labels().Get(mainImageCIDKey))
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				if data.Labels().Get(mainImageCIDKey) != "" {
					helpers.Anyhow("rmi", "-f", data.Labels().Get(mainImageCIDKey))
				}
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", data.Labels().Get(mainImageCIDKey), "ls", "/.stargz-snapshotter")
			},
			Expected: test.Expects(0, nil, expect.Match(regexp.MustCompile("sha256:.*[.]json[\n]"))),
		},
		{
			Description: "with commit and push",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				data.Labels().Set(mainImageCIDKey, pushToIPFS(helpers, testutil.CommonImage))
				helpers.Ensure("pull", "--quiet", "ipfs://"+data.Labels().Get(mainImageCIDKey))

				// Run a container that does modify something, then commit and push it
				helpers.Ensure("run", "--name", data.Identifier("commit-container"), data.Labels().Get(mainImageCIDKey), "sh", "-c", "--", "echo hello > /hello")
				helpers.Ensure("commit", data.Identifier("commit-container"), data.Identifier("commit-image"))
				data.Labels().Set(transformedImageCIDKey, pushToIPFS(helpers, data.Identifier("commit-image")))

				// Clean-up
				helpers.Ensure("rm", data.Identifier("commit-container"))
				helpers.Ensure("rmi", data.Identifier("commit-image"))

				// Pull back the committed image
				helpers.Ensure("pull", "--quiet", "ipfs://"+data.Labels().Get(transformedImageCIDKey))
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier("commit-container"))
				helpers.Anyhow("rmi", "-f", data.Identifier("commit-image"))
				if data.Labels().Get(mainImageCIDKey) != "" {
					helpers.Anyhow("rmi", "-f", data.Labels().Get(mainImageCIDKey))
					helpers.Anyhow("rmi", "-f", data.Labels().Get(transformedImageCIDKey))
				}
			},

			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", data.Labels().Get(transformedImageCIDKey), "cat", "/hello")
			},

			Expected: test.Expects(0, nil, expect.Equals("hello\n")),
		},
		{
			Description: "with commit and push, stargz lazy pulling",
			NoParallel:  true,
			Require: require.All(
				nerdtest.Stargz,
				nerdtest.NerdctlNeedsFixing("https://github.com/containerd/nerdctl/issues/3475"),
			),
			Setup: func(data test.Data, helpers test.Helpers) {
				data.Labels().Set(mainImageCIDKey, pushToIPFS(helpers, testutil.CommonImage, "--estargz"))
				helpers.Ensure("pull", "--quiet", "ipfs://"+data.Labels().Get(mainImageCIDKey))

				// Run a container that does modify something, then commit and push it
				helpers.Ensure("run", "--name", data.Identifier("commit-container"), data.Labels().Get(mainImageCIDKey), "sh", "-c", "--", "echo hello > /hello")
				helpers.Ensure("commit", data.Identifier("commit-container"), data.Identifier("commit-image"))
				data.Labels().Set(transformedImageCIDKey, pushToIPFS(helpers, data.Identifier("commit-image")))

				// Clean-up
				helpers.Ensure("rm", data.Identifier("commit-container"))
				helpers.Ensure("rmi", data.Identifier("commit-image"))

				// Pull back the image
				helpers.Ensure("pull", "--quiet", "ipfs://"+data.Labels().Get(transformedImageCIDKey))
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier("commit-container"))
				helpers.Anyhow("rmi", "-f", data.Identifier("commit-image"))
				if data.Labels().Get(mainImageCIDKey) != "" {
					helpers.Anyhow("rmi", "-f", data.Labels().Get(mainImageCIDKey))
					helpers.Anyhow("rmi", "-f", data.Labels().Get(transformedImageCIDKey))
				}
			},

			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", data.Labels().Get(transformedImageCIDKey), "sh", "-c", "--", "cat /hello && ls /.stargz-snapshotter")
			},

			Expected: test.Expects(0, nil, expect.Match(regexp.MustCompile("hello[\n]sha256:.*[.]json[\n]"))),
		},
		{
			Description: "with encryption",
			NoParallel:  true,
			Require:     require.Binary("openssl"),
			Setup: func(data test.Data, helpers test.Helpers) {
				data.Labels().Set(mainImageCIDKey, pushToIPFS(helpers, testutil.CommonImage))
				helpers.Ensure("pull", "--quiet", "ipfs://"+data.Labels().Get(mainImageCIDKey))

				// Prep a key pair
				pri, pub := nerdtest.GenerateJWEKeyPair(data, helpers)
				data.Labels().Set("prv", pri)
				data.Labels().Set("pub", pub)

				// Encrypt the image, and verify it is encrypted
				helpers.Ensure("image", "encrypt", "--recipient=jwe:"+pub, data.Labels().Get(mainImageCIDKey), data.Identifier("encrypted"))
				cmd := helpers.Command("image", "inspect", "--mode=native", "--format={{len .Index.Manifests}}", data.Identifier("encrypted"))
				cmd.Run(&test.Expected{
					Output: expect.Equals("1\n"),
				})
				cmd = helpers.Command("image", "inspect", "--mode=native", "--format={{json (index .Manifest.Layers 0) }}", data.Identifier("encrypted"))
				cmd.Run(&test.Expected{
					Output: expect.Contains("org.opencontainers.image.enc.keys.jwe"),
				})

				// Push the encrypted image and save the CID
				data.Labels().Set(transformedImageCIDKey, pushToIPFS(helpers, data.Identifier("encrypted")))

				// Remove both images locally
				helpers.Ensure("rmi", "-f", data.Labels().Get(mainImageCIDKey))
				helpers.Ensure("rmi", "-f", data.Labels().Get(transformedImageCIDKey))

				// Pull back without unpacking
				helpers.Ensure("pull", "--quiet", "--unpack=false", "ipfs://"+data.Labels().Get(transformedImageCIDKey))
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				if data.Labels().Get(mainImageCIDKey) != "" {
					helpers.Anyhow("rmi", "-f", data.Labels().Get(mainImageCIDKey))
					helpers.Anyhow("rmi", "-f", data.Labels().Get(transformedImageCIDKey))
				}
			},
			SubTests: []*test.Case{
				{
					Description: "decrypt with pub key does not work",
					Cleanup: func(data test.Data, helpers test.Helpers) {
						helpers.Anyhow("rm", "-f", data.Identifier("decrypted"))
					},
					Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
						return helpers.Command("image", "decrypt", "--key="+data.Labels().Get("pub"), data.Labels().Get(transformedImageCIDKey), data.Identifier("decrypted"))
					},
					Expected: test.Expects(1, nil, nil),
				},
				{
					Description: "decrypt with priv key does work",
					Cleanup: func(data test.Data, helpers test.Helpers) {
						helpers.Anyhow("rm", "-f", data.Identifier("decrypted"))
					},
					Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
						return helpers.Command("image", "decrypt", "--key="+data.Labels().Get("prv"), data.Labels().Get(transformedImageCIDKey), data.Identifier("decrypted"))
					},
					Expected: test.Expects(0, nil, nil),
				},
			},
		},
	}

	testCase.Run(t)
}
