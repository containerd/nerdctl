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
	"time"

	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/registry"
)

func TestImageConvert(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Require: require.All(
			// FIXME: windows does not support stargz
			require.Not(require.Windows),
			require.Not(nerdtest.Docker),
		),
		NoParallel: true,
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("pull", "--quiet", "--all-platforms", testutil.CommonImage)
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
			{
				Description: "soci",
				Require: require.All(
					require.Not(nerdtest.Docker),
					nerdtest.Soci,
					nerdtest.SociVersion("0.10.0"),
				),
				Setup: func(data test.Data, helpers test.Helpers) {
					// Clean up any existing SOCI indices to avoid stale ztoc data
					helpers.Anyhow("rmi", "-f", testutil.CommonImage)
					helpers.Anyhow("system", "prune", "--force")
					helpers.Ensure("pull", "--quiet", "--all-platforms", testutil.CommonImage)
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier("converted-image"))
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("image", "convert", "--soci",
						"--soci-span-size", "2097152",
						"--soci-min-layer-size", "0",
						testutil.CommonImage, data.Identifier("converted-image"))
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "soci with all-platforms",
				Require: require.All(
					require.Not(nerdtest.Docker),
					nerdtest.Soci,
					nerdtest.SociVersion("0.10.0"),
				),
				Setup: func(data test.Data, helpers test.Helpers) {
					// Clean up any existing SOCI indices to avoid stale ztoc data
					helpers.Anyhow("rmi", "-f", testutil.CommonImage)
					helpers.Anyhow("system", "prune", "--force")
					helpers.Ensure("pull", "--quiet", "--all-platforms", testutil.CommonImage)
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", data.Identifier("converted-image"))
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("image", "convert", "--soci", "--all-platforms",
						"--soci-span-size", "2097152",
						"--soci-min-layer-size", "0",
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

	var reg *registry.Server

	// It is unclear what is problematic here, but we use the kernel version to discriminate against EL
	// See: https://github.com/containerd/nerdctl/issues/4332
	testutil.RequireKernelVersion(t, ">= 6.0.0-0")

	testCase := &test.Case{
		Require: require.All(
			require.Linux,
			require.Binary("nydus-image"),
			require.Binary("nydusify"),
			require.Binary("nydusd"),
			require.Not(nerdtest.Docker),
			nerdtest.Rootful,
			nerdtest.Registry,
		),
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("pull", "--quiet", testutil.CommonImage)
			reg = nerdtest.RegistryWithNoAuth(data, helpers, 0, false)
			reg.Setup(data, helpers)

			data.Labels().Set(remoteImageKey, fmt.Sprintf("%s:%d/nydusd-image:test", "localhost", reg.Port))
			helpers.Ensure("image", "convert", "--nydus", "--oci", testutil.CommonImage, data.Identifier("converted-image"))
			helpers.Ensure("tag", data.Identifier("converted-image"), data.Labels().Get(remoteImageKey))
			helpers.Ensure("push", data.Labels().Get(remoteImageKey))
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier("converted-image"))
			if reg != nil {
				reg.Cleanup(data, helpers)
				helpers.Anyhow("rmi", "-f", data.Labels().Get(remoteImageKey))
			}
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			cmd := helpers.Custom("nydusify",
				"check",
				"--work-dir",
				data.Temp().Dir("nydusify-temp"),
				"--source",
				testutil.CommonImage,
				"--target",
				data.Labels().Get(remoteImageKey),
				"--source-insecure",
				"--target-insecure",
			)
			cmd.WithTimeout(30 * time.Second)
			return cmd
		},
		Expected: test.Expects(0, nil, nil),
	}

	testCase.Run(t)
}
