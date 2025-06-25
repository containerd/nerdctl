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
	"errors"
	"fmt"
	"net/http"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/registry"
)

func TestPush(t *testing.T) {
	nerdtest.Setup()

	var registryNoAuthHTTPRandom, registryNoAuthHTTPDefault, registryTokenAuthHTTPSRandom *registry.Server
	var tokenServer *registry.TokenAuthServer

	testCase := &test.Case{
		Require: require.All(
			require.Linux,
			nerdtest.Registry,
		),

		Setup: func(data test.Data, helpers test.Helpers) {
			registryNoAuthHTTPRandom = nerdtest.RegistryWithNoAuth(data, helpers, 0, false)
			registryNoAuthHTTPRandom.Setup(data, helpers)
			registryNoAuthHTTPDefault = nerdtest.RegistryWithNoAuth(data, helpers, 80, false)
			registryNoAuthHTTPDefault.Setup(data, helpers)
			registryTokenAuthHTTPSRandom, tokenServer = nerdtest.RegistryWithTokenAuth(data, helpers, "admin", "badmin", 0, true)
			tokenServer.Setup(data, helpers)
			registryTokenAuthHTTPSRandom.Setup(data, helpers)
		},

		Cleanup: func(data test.Data, helpers test.Helpers) {
			if registryNoAuthHTTPRandom != nil {
				registryNoAuthHTTPRandom.Cleanup(data, helpers)
			}
			if registryNoAuthHTTPDefault != nil {
				registryNoAuthHTTPDefault.Cleanup(data, helpers)
			}
			if registryTokenAuthHTTPSRandom != nil {
				registryTokenAuthHTTPSRandom.Cleanup(data, helpers)
			}
			if tokenServer != nil {
				tokenServer.Cleanup(data, helpers)
			}
		},

		SubTests: []*test.Case{
			{
				Description: "plain http",
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("pull", "--quiet", testutil.CommonImage)
					testImageRef := fmt.Sprintf("%s:%d/%s",
						registryNoAuthHTTPRandom.IP.String(), registryNoAuthHTTPRandom.Port, data.Identifier())
					data.Labels().Set("testImageRef", testImageRef)
					helpers.Ensure("tag", testutil.CommonImage, testImageRef)
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					if data.Labels().Get("testImageRef") != "" {
						helpers.Anyhow("rmi", "-f", data.Labels().Get("testImageRef"))
					}
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("push", data.Labels().Get("testImageRef"))
				},
				Expected: test.Expects(1, []error{errors.New("server gave HTTP response to HTTPS client")}, nil),
			},
			{
				Description: "plain http with insecure",
				Require:     require.Not(nerdtest.Docker),
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("pull", "--quiet", testutil.CommonImage)
					testImageRef := fmt.Sprintf("%s:%d/%s",
						registryNoAuthHTTPRandom.IP.String(), registryNoAuthHTTPRandom.Port, data.Identifier())
					data.Labels().Set("testImageRef", testImageRef)
					helpers.Ensure("tag", testutil.CommonImage, testImageRef)
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					if data.Labels().Get("testImageRef") != "" {
						helpers.Anyhow("rmi", "-f", data.Labels().Get("testImageRef"))
					}
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("push", "--insecure-registry", data.Labels().Get("testImageRef"))
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "plain http with localhost",
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("pull", "--quiet", testutil.CommonImage)
					testImageRef := fmt.Sprintf("%s:%d/%s",
						"127.0.0.1", registryNoAuthHTTPRandom.Port, data.Identifier())
					data.Labels().Set("testImageRef", testImageRef)
					helpers.Ensure("tag", testutil.CommonImage, testImageRef)
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("push", data.Labels().Get("testImageRef"))
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "plain http with insecure, default port",
				Require:     require.Not(nerdtest.Docker),
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("pull", "--quiet", testutil.CommonImage)
					testImageRef := fmt.Sprintf("%s/%s",
						registryNoAuthHTTPDefault.IP.String(), data.Identifier())
					data.Labels().Set("testImageRef", testImageRef)
					helpers.Ensure("tag", testutil.CommonImage, testImageRef)
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					if data.Labels().Get("testImageRef") != "" {
						helpers.Anyhow("rmi", "-f", data.Labels().Get("testImageRef"))
					}
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("push", "--insecure-registry", data.Labels().Get("testImageRef"))
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "with insecure, with login",
				Require:     require.Not(nerdtest.Docker),
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("pull", "--quiet", testutil.CommonImage)
					testImageRef := fmt.Sprintf("%s:%d/%s",
						registryTokenAuthHTTPSRandom.IP.String(), registryTokenAuthHTTPSRandom.Port, data.Identifier())
					data.Labels().Set("testImageRef", testImageRef)
					helpers.Ensure("tag", testutil.CommonImage, testImageRef)
					helpers.Ensure("--insecure-registry", "login", "-u", "admin", "-p", "badmin",
						fmt.Sprintf("%s:%d", registryTokenAuthHTTPSRandom.IP.String(), registryTokenAuthHTTPSRandom.Port))

				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					if data.Labels().Get("testImageRef") != "" {
						helpers.Anyhow("rmi", "-f", data.Labels().Get("testImageRef"))
					}
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("push", "--insecure-registry", data.Labels().Get("testImageRef"))
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "with hosts dir, with login",
				Require:     require.Not(nerdtest.Docker),
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("pull", "--quiet", testutil.CommonImage)
					testImageRef := fmt.Sprintf("%s:%d/%s",
						registryTokenAuthHTTPSRandom.IP.String(), registryTokenAuthHTTPSRandom.Port, data.Identifier())
					data.Labels().Set("testImageRef", testImageRef)
					helpers.Ensure("tag", testutil.CommonImage, testImageRef)
					helpers.Ensure("--hosts-dir", registryTokenAuthHTTPSRandom.HostsDir, "login", "-u", "admin", "-p", "badmin",
						fmt.Sprintf("%s:%d", registryTokenAuthHTTPSRandom.IP.String(), registryTokenAuthHTTPSRandom.Port))

				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					if data.Labels().Get("testImageRef") != "" {
						helpers.Anyhow("rmi", "-f", data.Labels().Get("testImageRef"))
					}
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("push", "--hosts-dir", registryTokenAuthHTTPSRandom.HostsDir, data.Labels().Get("testImageRef"))
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "non distributable artifacts",
				Require:     require.Not(nerdtest.Docker),
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("pull", "--quiet", testutil.NonDistBlobImage)
					testImageRef := fmt.Sprintf("%s:%d/%s",
						registryNoAuthHTTPRandom.IP.String(), registryNoAuthHTTPRandom.Port, data.Identifier())
					data.Labels().Set("testImageRef", testImageRef)
					helpers.Ensure("tag", testutil.NonDistBlobImage, testImageRef)
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					if data.Labels().Get("testImageRef") != "" {
						helpers.Anyhow("rmi", "-f", data.Labels().Get("testImageRef"))
					}
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("push", "--insecure-registry", data.Labels().Get("testImageRef"))
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, t tig.T) {
							blobURL := fmt.Sprintf("http://%s:%d/v2/%s/blobs/%s", registryNoAuthHTTPRandom.IP.String(), registryNoAuthHTTPRandom.Port, data.Identifier(), testutil.NonDistBlobDigest)
							resp, err := http.Get(blobURL)
							assert.Assert(t, err, "error making http request")
							if resp.Body != nil {
								_ = resp.Body.Close()
							}
							assert.Equal(t, resp.StatusCode, http.StatusNotFound, "non-distributable blob should not be available")
						},
					}
				},
			},
			{
				Description: "non distributable artifacts (with)",
				Require:     require.Not(nerdtest.Docker),
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("pull", "--quiet", testutil.NonDistBlobImage)
					testImageRef := fmt.Sprintf("%s:%d/%s",
						registryNoAuthHTTPRandom.IP.String(), registryNoAuthHTTPRandom.Port, data.Identifier())
					data.Labels().Set("testImageRef", testImageRef)
					helpers.Ensure("tag", testutil.NonDistBlobImage, testImageRef)
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					if data.Labels().Get("testImageRef") != "" {
						helpers.Anyhow("rmi", "-f", data.Labels().Get("testImageRef"))
					}
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("push", "--insecure-registry", "--allow-nondistributable-artifacts", data.Labels().Get("testImageRef"))
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, t tig.T) {
							blobURL := fmt.Sprintf("http://%s:%d/v2/%s/blobs/%s", registryNoAuthHTTPRandom.IP.String(), registryNoAuthHTTPRandom.Port, data.Identifier(), testutil.NonDistBlobDigest)
							resp, err := http.Get(blobURL)
							assert.Assert(t, err, "error making http request")
							if resp.Body != nil {
								_ = resp.Body.Close()
							}
							assert.Equal(t, resp.StatusCode, http.StatusOK, "non-distributable blob should be available")
						},
					}
				},
			},
			{
				Description: "soci",
				Require: require.All(
					nerdtest.Soci,
					require.Not(nerdtest.Docker),
				),
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("pull", "--quiet", testutil.UbuntuImage)
					testImageRef := fmt.Sprintf("%s:%d/%s",
						registryNoAuthHTTPRandom.IP.String(), registryNoAuthHTTPRandom.Port, data.Identifier())
					data.Labels().Set("testImageRef", testImageRef)
					helpers.Ensure("tag", testutil.UbuntuImage, testImageRef)
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					if data.Labels().Get("testImageRef") != "" {
						helpers.Anyhow("rmi", "-f", data.Labels().Get("testImageRef"))
					}
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("push", "--snapshotter=soci", "--insecure-registry", "--soci-span-size=2097152", "--soci-min-layer-size=20971520", data.Labels().Get("testImageRef"))
				},
				Expected: test.Expects(0, nil, nil),
			},
		},
	}
	testCase.Run(t)
}
