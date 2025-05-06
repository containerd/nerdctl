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
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/testregistry"
)

func TestImageConvert(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Require: require.All(
			// FIXME: windows does not support stargz
			require.Not(require.Windows),
			require.Not(nerdtest.Docker),
		),
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("pull", "--quiet", testutil.CommonImage)
		},
		SubTests: []*test.Case{
			{
				Description: "esgz",
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier("converted-image"))
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("image", "convert", "--oci", "--estargz",
						testutil.CommonImage, data.Identifier("converted-image"))
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "nydus",
				Require: require.All(
					require.Binary("nydus-image"),
				),
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier("converted-image"))
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("image", "convert", "--oci", "--nydus",
						testutil.CommonImage, data.Identifier("converted-image"))
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "zstd",
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier("converted-image"))
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("image", "convert", "--oci", "--zstd", "--zstd-compression-level", "3",
						testutil.CommonImage, data.Identifier("converted-image"))
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "zstdchunked",
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier("converted-image"))
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("image", "convert", "--oci", "--zstdchunked", "--zstdchunked-compression-level", "3",
						testutil.CommonImage, data.Identifier("converted-image"))
				},
				Expected: test.Expects(0, nil, nil),
			},
		},
	}

	testCase.Run(t)

}

func TestImageConvertNydusVerify(t *testing.T) {
	nerdtest.Setup()

	const remoteImageKey = "remoteImageKey"

	var registry *testregistry.RegistryServer

	testCase := &test.Case{
		Require: require.All(
			require.Linux,
			require.Binary("nydus-image"),
			require.Binary("nydusify"),
			require.Binary("nydusd"),
			require.Not(nerdtest.Docker),
			nerdtest.Rootful,
		),
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("pull", "--quiet", testutil.CommonImage)
			base := testutil.NewBase(t)
			registry = testregistry.NewWithNoAuth(base, 0, false)
			data.Labels().Set(remoteImageKey, fmt.Sprintf("%s:%d/nydusd-image:test", "localhost", registry.Port))
			helpers.Ensure("image", "convert", "--nydus", "--oci", testutil.CommonImage, data.Identifier("converted-image"))
			helpers.Ensure("tag", data.Identifier("converted-image"), data.Labels().Get(remoteImageKey))
			helpers.Ensure("push", data.Labels().Get(remoteImageKey))
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier("converted-image"))
			if registry != nil {
				registry.Cleanup(nil)
				helpers.Anyhow("rmi", "-f", data.Labels().Get(remoteImageKey))
			}
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Custom("nydusify",
				"check",
				"--source",
				testutil.CommonImage,
				"--target",
				data.Labels().Get(remoteImageKey),
				"--source-insecure",
				"--target-insecure",
			)
		},
		Expected: test.Expects(0, nil, nil),
	}

	testCase.Run(t)
}
