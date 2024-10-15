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
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	testhelpers "github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func pushToIPFS(helpers test.Helpers, name string, opts ...string) string {
	var ipfsCID string
	cmd := helpers.Command("push", "ipfs://"+name)
	cmd.WithArgs(opts...)
	cmd.Run(&test.Expected{
		Output: func(stdout string, info string, t *testing.T) {
			lines := strings.Split(stdout, "\n")
			assert.Equal(t, len(lines) >= 2, true)
			ipfsCID = lines[len(lines)-2]
		},
	})
	return ipfsCID
}

func TestIPFSNerdctlRegistry(t *testing.T) {
	testCase := nerdtest.Setup()

	// FIXME: this is bad and likely to collide with other tests
	const listenAddr = "localhost:5555"

	const ipfsImageURLKey = "ipfsImageURLKey"

	var ipfsServer test.TestableCommand

	testCase.Require = test.Require(
		test.Linux,
		test.Not(nerdtest.Docker),
		nerdtest.IPFS,
	)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("pull", "--quiet", testutil.AlpineImage)

		// Start a local ipfs backed registry
		ipfsServer = helpers.Command("ipfs", "registry", "serve", "--listen-registry", listenAddr)
		// Once foregrounded, do not wait for it more than a second
		ipfsServer.Background(1 * time.Second)
		// Apparently necessary to let it start...
		time.Sleep(time.Second)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if ipfsServer != nil {
			// Close the server once done
			ipfsServer.Run(nil)
		}
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "with default snapshotter",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				data.Set(ipfsImageURLKey, listenAddr+"/ipfs/"+pushToIPFS(helpers, testutil.AlpineImage))
				helpers.Ensure("pull", data.Get(ipfsImageURLKey))
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				if data.Get(ipfsImageURLKey) != "" {
					helpers.Anyhow("rmi", "-f", data.Get(ipfsImageURLKey))
				}
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", data.Get(ipfsImageURLKey), "echo", "hello")
			},
			Expected: test.Expects(0, nil, test.Equals("hello\n")),
		},
		{
			Description: "with stargz snapshotterr",
			NoParallel:  true,
			Require:     nerdtest.Stargz,
			Setup: func(data test.Data, helpers test.Helpers) {
				data.Set(ipfsImageURLKey, listenAddr+"/ipfs/"+pushToIPFS(helpers, testutil.AlpineImage, "--estargz"))
				helpers.Ensure("pull", data.Get(ipfsImageURLKey))
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				if data.Get(ipfsImageURLKey) != "" {
					helpers.Anyhow("rmi", "-f", data.Get(ipfsImageURLKey))
				}
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", data.Get(ipfsImageURLKey), "ls", "/.stargz-snapshotter")
			},
			Expected: test.Expects(0, nil, test.Match(regexp.MustCompile("sha256:.*[.]json[\n]"))),
		},
		{
			Description: "with build",
			NoParallel:  true,
			Require:     nerdtest.Build,
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rmi", "-f", data.Identifier("built-image"))
				if data.Get(ipfsImageURLKey) != "" {
					helpers.Anyhow("rmi", "-f", data.Get(ipfsImageURLKey))
				}
			},
			Setup: func(data test.Data, helpers test.Helpers) {
				data.Set(ipfsImageURLKey, listenAddr+"/ipfs/"+pushToIPFS(helpers, testutil.AlpineImage))

				dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, data.Get(ipfsImageURLKey))

				buildCtx := testhelpers.CreateBuildContext(t, dockerfile)

				helpers.Ensure("build", "-t", data.Identifier("built-image"), buildCtx)
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", data.Identifier("built-image"))
			},
			Expected: test.Expects(0, nil, test.Equals("nerdctl-build-test-string\n")),
		},
	}

	testCase.Run(t)
}
