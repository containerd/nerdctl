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

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
	"github.com/containerd/nerdctl/v2/pkg/testutil/testregistry"
)

func TestImageConvert(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Description: "Test image conversion",
		Require: test.Require(
			test.Not(test.Windows),
			test.Not(nerdtest.Docker),
		),
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("pull", testutil.CommonImage)
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
				Require: test.Require(
					test.Binary("nydus-image"),
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
		Description: "TestImageConvertNydusVerify",
		Require: test.Require(
			test.Linux,
			test.Binary("nydus-image"),
			test.Binary("nydusify"),
			test.Binary("nydusd"),
			test.Not(nerdtest.Docker),
			nerdtest.Rootful,
		),
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("pull", testutil.CommonImage)
			base := testutil.NewBase(t)
			registry = testregistry.NewWithNoAuth(base, 0, false)
			data.Set(remoteImageKey, fmt.Sprintf("%s:%d/nydusd-image:test", "localhost", registry.Port))
			helpers.Ensure("image", "convert", "--nydus", "--oci", testutil.CommonImage, data.Identifier("converted-image"))
			helpers.Ensure("tag", data.Identifier("converted-image"), data.Get(remoteImageKey))
			helpers.Ensure("push", data.Get(remoteImageKey))
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier("converted-image"))
			if registry != nil {
				registry.Cleanup(nil)
				helpers.Anyhow("rmi", "-f", data.Get(remoteImageKey))
			}
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Custom("nydusify",
				"check",
				"--source",
				testutil.CommonImage,
				"--target",
				data.Get(remoteImageKey),
				"--source-insecure",
				"--target-insecure",
			)
		},
		Expected: test.Expects(0, nil, nil),
	}

	testCase.Run(t)
}
