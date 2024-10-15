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
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
	"github.com/containerd/nerdctl/v2/pkg/testutil/testregistry"
)

func TestPush(t *testing.T) {
	nerdtest.Setup()

	var registryNoAuthHTTPRandom, registryNoAuthHTTPDefault, registryTokenAuthHTTPSRandom *testregistry.RegistryServer

	testCase := &test.Case{
		Description: "Test push",

		Require: test.Linux,

		Setup: func(data test.Data, helpers test.Helpers) {
			base := testutil.NewBase(t)
			registryNoAuthHTTPRandom = testregistry.NewWithNoAuth(base, 0, false)
			registryNoAuthHTTPDefault = testregistry.NewWithNoAuth(base, 80, false)
			registryTokenAuthHTTPSRandom = testregistry.NewWithTokenAuth(base, "admin", "badmin", 0, true)
		},

		Cleanup: func(data test.Data, helpers test.Helpers) {
			if registryNoAuthHTTPRandom != nil {
				registryNoAuthHTTPRandom.Cleanup(nil)
			}
			if registryNoAuthHTTPDefault != nil {
				registryNoAuthHTTPDefault.Cleanup(nil)
			}
			if registryTokenAuthHTTPSRandom != nil {
				registryTokenAuthHTTPSRandom.Cleanup(nil)
			}
		},

		SubTests: []*test.Case{
			{
				Description: "plain http",
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("pull", "--quiet", testutil.CommonImage)
					testImageRef := fmt.Sprintf("%s:%d/%s:%s",
						registryNoAuthHTTPRandom.IP.String(), registryNoAuthHTTPRandom.Port, data.Identifier(), strings.Split(testutil.CommonImage, ":")[1])
					data.Set("testImageRef", testImageRef)
					helpers.Ensure("tag", testutil.CommonImage, testImageRef)
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					if data.Get("testImageRef") != "" {
						helpers.Anyhow("rmi", "-f", data.Get("testImageRef"))
					}
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("push", data.Get("testImageRef"))
				},
				Expected: test.Expects(1, []error{errors.New("server gave HTTP response to HTTPS client")}, nil),
			},
			{
				Description: "plain http with insecure",
				Require:     test.Not(nerdtest.Docker),
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("pull", "--quiet", testutil.CommonImage)
					testImageRef := fmt.Sprintf("%s:%d/%s:%s",
						registryNoAuthHTTPRandom.IP.String(), registryNoAuthHTTPRandom.Port, data.Identifier(), strings.Split(testutil.CommonImage, ":")[1])
					data.Set("testImageRef", testImageRef)
					helpers.Ensure("tag", testutil.CommonImage, testImageRef)
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					if data.Get("testImageRef") != "" {
						helpers.Anyhow("rmi", "-f", data.Get("testImageRef"))
					}
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("push", "--insecure-registry", data.Get("testImageRef"))
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "plain http with localhost",
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("pull", "--quiet", testutil.CommonImage)
					testImageRef := fmt.Sprintf("%s:%d/%s:%s",
						"127.0.0.1", registryNoAuthHTTPRandom.Port, data.Identifier(), strings.Split(testutil.CommonImage, ":")[1])
					data.Set("testImageRef", testImageRef)
					helpers.Ensure("tag", testutil.CommonImage, testImageRef)
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("push", data.Get("testImageRef"))
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "plain http with insecure, default port",
				Require:     test.Not(nerdtest.Docker),
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("pull", "--quiet", testutil.CommonImage)
					testImageRef := fmt.Sprintf("%s/%s:%s",
						registryNoAuthHTTPDefault.IP.String(), data.Identifier(), strings.Split(testutil.CommonImage, ":")[1])
					data.Set("testImageRef", testImageRef)
					helpers.Ensure("tag", testutil.CommonImage, testImageRef)
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					if data.Get("testImageRef") != "" {
						helpers.Anyhow("rmi", "-f", data.Get("testImageRef"))
					}
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("push", "--insecure-registry", data.Get("testImageRef"))
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "with insecure, with login",
				Require:     test.Not(nerdtest.Docker),
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("pull", "--quiet", testutil.CommonImage)
					testImageRef := fmt.Sprintf("%s:%d/%s:%s",
						registryTokenAuthHTTPSRandom.IP.String(), registryTokenAuthHTTPSRandom.Port, data.Identifier(), strings.Split(testutil.CommonImage, ":")[1])
					data.Set("testImageRef", testImageRef)
					helpers.Ensure("tag", testutil.CommonImage, testImageRef)
					helpers.Ensure("--insecure-registry", "login", "-u", "admin", "-p", "badmin",
						fmt.Sprintf("%s:%d", registryTokenAuthHTTPSRandom.IP.String(), registryTokenAuthHTTPSRandom.Port))

				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					if data.Get("testImageRef") != "" {
						helpers.Anyhow("rmi", "-f", data.Get("testImageRef"))
					}
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("push", "--insecure-registry", data.Get("testImageRef"))
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "with hosts dir, with login",
				Require:     test.Not(nerdtest.Docker),
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("pull", "--quiet", testutil.CommonImage)
					testImageRef := fmt.Sprintf("%s:%d/%s:%s",
						registryTokenAuthHTTPSRandom.IP.String(), registryTokenAuthHTTPSRandom.Port, data.Identifier(), strings.Split(testutil.CommonImage, ":")[1])
					data.Set("testImageRef", testImageRef)
					helpers.Ensure("tag", testutil.CommonImage, testImageRef)
					helpers.Ensure("--hosts-dir", registryTokenAuthHTTPSRandom.HostsDir, "login", "-u", "admin", "-p", "badmin",
						fmt.Sprintf("%s:%d", registryTokenAuthHTTPSRandom.IP.String(), registryTokenAuthHTTPSRandom.Port))

				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					if data.Get("testImageRef") != "" {
						helpers.Anyhow("rmi", "-f", data.Get("testImageRef"))
					}
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("push", "--hosts-dir", registryTokenAuthHTTPSRandom.HostsDir, data.Get("testImageRef"))
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "non distributable artifacts",
				Require:     test.Not(nerdtest.Docker),
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("pull", "--quiet", testutil.NonDistBlobImage)
					testImageRef := fmt.Sprintf("%s:%d/%s:%s",
						registryNoAuthHTTPRandom.IP.String(), registryNoAuthHTTPRandom.Port, data.Identifier(), strings.Split(testutil.NonDistBlobImage, ":")[1])
					data.Set("testImageRef", testImageRef)
					helpers.Ensure("tag", testutil.NonDistBlobImage, testImageRef)
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					if data.Get("testImageRef") != "" {
						helpers.Anyhow("rmi", "-f", data.Get("testImageRef"))
					}
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("push", "--insecure-registry", data.Get("testImageRef"))
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							blobURL := fmt.Sprintf("http://%s:%d/v2/%s/blobs/%s", registryNoAuthHTTPRandom.IP.String(), registryNoAuthHTTPRandom.Port, data.Identifier(), testutil.NonDistBlobDigest)
							resp, err := http.Get(blobURL)
							assert.Assert(t, err, "error making http request")
							if resp.Body != nil {
								resp.Body.Close()
							}
							assert.Equal(t, resp.StatusCode, http.StatusNotFound, "non-distributable blob should not be available")
						},
					}
				},
			},
			{
				Description: "non distributable artifacts (with)",
				Require:     test.Not(nerdtest.Docker),
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("pull", "--quiet", testutil.NonDistBlobImage)
					testImageRef := fmt.Sprintf("%s:%d/%s:%s",
						registryNoAuthHTTPRandom.IP.String(), registryNoAuthHTTPRandom.Port, data.Identifier(), strings.Split(testutil.NonDistBlobImage, ":")[1])
					data.Set("testImageRef", testImageRef)
					helpers.Ensure("tag", testutil.NonDistBlobImage, testImageRef)
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					if data.Get("testImageRef") != "" {
						helpers.Anyhow("rmi", "-f", data.Get("testImageRef"))
					}
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("push", "--insecure-registry", "--allow-nondistributable-artifacts", data.Get("testImageRef"))
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							blobURL := fmt.Sprintf("http://%s:%d/v2/%s/blobs/%s", registryNoAuthHTTPRandom.IP.String(), registryNoAuthHTTPRandom.Port, data.Identifier(), testutil.NonDistBlobDigest)
							resp, err := http.Get(blobURL)
							assert.Assert(t, err, "error making http request")
							if resp.Body != nil {
								resp.Body.Close()
							}
							assert.Equal(t, resp.StatusCode, http.StatusOK, "non-distributable blob should be available")
						},
					}
				},
			},
			{
				Description: "soci",
				Require: test.Require(
					nerdtest.Soci,
					test.Not(nerdtest.Docker),
				),
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("pull", "--quiet", testutil.UbuntuImage)
					testImageRef := fmt.Sprintf("%s:%d/%s:%s",
						registryNoAuthHTTPRandom.IP.String(), registryNoAuthHTTPRandom.Port, data.Identifier(), strings.Split(testutil.UbuntuImage, ":")[1])
					data.Set("testImageRef", testImageRef)
					helpers.Ensure("tag", testutil.UbuntuImage, testImageRef)
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					if data.Get("testImageRef") != "" {
						helpers.Anyhow("rmi", "-f", data.Get("testImageRef"))
					}
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("push", "--snapshotter=soci", "--insecure-registry", "--soci-span-size=2097152", "--soci-min-layer-size=20971520", data.Get("testImageRef"))
				},
				Expected: test.Expects(0, nil, nil),
			},
		},
	}
	testCase.Run(t)
}
