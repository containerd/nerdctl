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
	"fmt"
	"regexp"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/registry"
)

func TestIPFSAddrWithKubo(t *testing.T) {
	testCase := nerdtest.Setup()

	const mainImageCIDKey = "mainImagemainImageCIDKey"
	const ipfsAddrKey = "ipfsAddrKey"

	var ipfsRegistry *registry.Server

	testCase.Require = require.All(
		require.Linux,
		require.Not(nerdtest.Docker),
		nerdtest.Registry,
		nerdtest.Private,
	)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("pull", "--quiet", testutil.CommonImage)

		ipfsRegistry = registry.NewKuboRegistry(data, helpers, t, nil, 0, nil)
		ipfsRegistry.Setup(data, helpers)
		ipfsAddr := fmt.Sprintf("/ip4/%s/tcp/%d", ipfsRegistry.IP, ipfsRegistry.Port)
		data.Labels().Set(ipfsAddrKey, ipfsAddr)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if ipfsRegistry != nil {
			ipfsRegistry.Cleanup(data, helpers)
		}
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "with default snapshotter",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				ipfsCID := pushToIPFS(helpers, testutil.CommonImage, fmt.Sprintf("--ipfs-address=%s", data.Labels().Get(ipfsAddrKey)))
				helpers.Ensure("pull", "--quiet", "--ipfs-address", data.Labels().Get(ipfsAddrKey), "ipfs://"+ipfsCID)
				data.Labels().Set(mainImageCIDKey, ipfsCID)
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
				nerdtest.Private,
				nerdtest.NerdctlNeedsFixing("https://github.com/containerd/nerdctl/issues/3475"),
			),
			Setup: func(data test.Data, helpers test.Helpers) {
				ipfsCID := pushToIPFS(helpers, testutil.CommonImage, fmt.Sprintf("--ipfs-address=%s", data.Labels().Get(ipfsAddrKey)), "--estargz")
				helpers.Ensure("pull", "--quiet", "--ipfs-address", data.Labels().Get(ipfsAddrKey), "ipfs://"+ipfsCID)
				data.Labels().Set(mainImageCIDKey, ipfsCID)
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
	}

	testCase.Run(t)
}
