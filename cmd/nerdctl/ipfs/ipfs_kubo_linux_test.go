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

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/registry"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestIPFSAddrWithKubo(t *testing.T) {
	testCase := nerdtest.Setup()

	const mainImageCIDKey = "mainImagemainImageCIDKey"
	const ipfsAddrKey = "ipfsAddrKey"

	var ipfsRegistry *registry.Server

	testCase.Require = test.Require(
		test.Linux,
		test.Not(nerdtest.Docker),
		nerdtest.Registry,
		nerdtest.Private,
	)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("pull", "--quiet", testutil.AlpineImage)

		ipfsRegistry = registry.NewKuboRegistry(data, helpers, t, nil, 0, nil)
		ipfsRegistry.Setup(data, helpers)
		ipfsAddr := fmt.Sprintf("/ip4/%s/tcp/%d", ipfsRegistry.IP, ipfsRegistry.Port)
		data.Set(ipfsAddrKey, ipfsAddr)
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
				ipfsCID := pushToIPFS(helpers, testutil.AlpineImage, fmt.Sprintf("--ipfs-address=%s", data.Get(ipfsAddrKey)))
				helpers.Ensure("pull", "--ipfs-address", data.Get(ipfsAddrKey), "ipfs://"+ipfsCID)
				data.Set(mainImageCIDKey, ipfsCID)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				if data.Get(mainImageCIDKey) != "" {
					helpers.Anyhow("rmi", data.Get(mainImageCIDKey))
				}
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", data.Get(mainImageCIDKey), "echo", "hello")
			},
			Expected: test.Expects(0, nil, test.Equals("hello\n")),
		},
		{
			Description: "with stargz snapshotter",
			NoParallel:  true,
			Require: test.Require(
				nerdtest.Stargz,
				nerdtest.Private,
				nerdtest.NerdctlNeedsFixing("https://github.com/containerd/nerdctl/issues/3475"),
			),
			Setup: func(data test.Data, helpers test.Helpers) {
				ipfsCID := pushToIPFS(helpers, testutil.AlpineImage, fmt.Sprintf("--ipfs-address=%s", data.Get(ipfsAddrKey)), "--estargz")
				helpers.Ensure("pull", "--ipfs-address", data.Get(ipfsAddrKey), "ipfs://"+ipfsCID)
				data.Set(mainImageCIDKey, ipfsCID)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				if data.Get(mainImageCIDKey) != "" {
					helpers.Anyhow("rmi", data.Get(mainImageCIDKey))
				}
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", data.Get(mainImageCIDKey), "ls", "/.stargz-snapshotter")
			},
			Expected: test.Expects(0, nil, test.Match(regexp.MustCompile("sha256:.*[.]json[\n]"))),
		},
	}

	testCase.Run(t)
}
