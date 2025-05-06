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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
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

	testCase.Require = require.All(
		require.Linux,
		require.Not(nerdtest.Docker),
		nerdtest.IPFS,
	)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("pull", "--quiet", testutil.CommonImage)

		// Start a local ipfs backed registry
		ipfsServer = helpers.Command("ipfs", "registry", "serve", "--listen-registry", listenAddr)
		// This should not take longer than that
		ipfsServer.WithTimeout(30 * time.Second)
		ipfsServer.Background()
		// Apparently necessary to let it start...
		time.Sleep(time.Second)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if ipfsServer != nil {
			// Close the server once done
			ipfsServer.Signal(os.Kill)
		}
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "with default snapshotter",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				data.Labels().Set(ipfsImageURLKey, listenAddr+"/ipfs/"+pushToIPFS(helpers, testutil.CommonImage))
				helpers.Ensure("pull", "--quiet", data.Labels().Get(ipfsImageURLKey))
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				if data.Labels().Get(ipfsImageURLKey) != "" {
					helpers.Anyhow("rmi", "-f", data.Labels().Get(ipfsImageURLKey))
				}
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", data.Labels().Get(ipfsImageURLKey), "echo", "hello")
			},
			Expected: test.Expects(0, nil, expect.Equals("hello\n")),
		},
		{
			Description: "with stargz snapshotterr",
			NoParallel:  true,
			Require:     nerdtest.Stargz,
			Setup: func(data test.Data, helpers test.Helpers) {
				data.Labels().Set(ipfsImageURLKey, listenAddr+"/ipfs/"+pushToIPFS(helpers, testutil.CommonImage, "--estargz"))
				helpers.Ensure("pull", "--quiet", data.Labels().Get(ipfsImageURLKey))
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				if data.Labels().Get(ipfsImageURLKey) != "" {
					helpers.Anyhow("rmi", "-f", data.Labels().Get(ipfsImageURLKey))
				}
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", data.Labels().Get(ipfsImageURLKey), "ls", "/.stargz-snapshotter")
			},
			Expected: test.Expects(0, nil, expect.Match(regexp.MustCompile("sha256:.*[.]json[\n]"))),
		},
		{
			Description: "with build",
			NoParallel:  true,
			Require:     nerdtest.Build,
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rmi", "-f", data.Identifier("built-image"))
				if data.Labels().Get(ipfsImageURLKey) != "" {
					helpers.Anyhow("rmi", "-f", data.Labels().Get(ipfsImageURLKey))
				}
			},
			Setup: func(data test.Data, helpers test.Helpers) {
				data.Labels().Set(ipfsImageURLKey, listenAddr+"/ipfs/"+pushToIPFS(helpers, testutil.CommonImage))

				dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, data.Labels().Get(ipfsImageURLKey))

				buildCtx := data.Temp().Path()
				err := os.WriteFile(filepath.Join(buildCtx, "Dockerfile"), []byte(dockerfile), 0o600)
				assert.NilError(helpers.T(), err)

				helpers.Ensure("build", "-t", data.Identifier("built-image"), buildCtx)
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", data.Identifier("built-image"))
			},
			Expected: test.Expects(0, nil, expect.Equals("nerdctl-build-test-string\n")),
		},
	}

	testCase.Run(t)
}
