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

package manifest

import (
	"errors"
	"fmt"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/registry"
)

func TestManifestPushErrors(t *testing.T) {
	testCase := nerdtest.Setup()
	invalidName := "invalid/name/with/special@chars"
	testCase.SubTests = []*test.Case{
		{
			Description: "require-one-argument",
			Command:     test.Command("manifest", "push", "arg1", "arg2"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
				}
			},
		},
		{
			Description: "invalid-list-name",
			Command:     test.Command("manifest", "push", invalidName),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New(data.Labels().Get("error"))},
				}
			},
			Data: test.WithLabels(map[string]string{
				"error": "invalid reference format",
			}),
		},
	}

	testCase.Run(t)
}

func TestManifestPush(t *testing.T) {
	nerdtest.Setup()

	var registryTokenAuthHTTPSRandom *registry.Server
	var tokenServer *registry.TokenAuthServer

	manifestRef := testutil.GetTestImageWithoutTag("alpine") + "@" + testutil.GetTestImageManifestDigest("alpine", "linux/amd64")
	expectedDigest := "sha256:5317ce2da263afa23570c692d62c1b01381285b2198b3ea9739ce64bec22aff2"

	testCase := &test.Case{
		Require: require.All(
			require.Linux,
			nerdtest.Registry,
		),
		Setup: func(data test.Data, helpers test.Helpers) {
			registryTokenAuthHTTPSRandom, tokenServer = nerdtest.RegistryWithTokenAuth(data, helpers, "admin", "badmin", 0, true)
			tokenServer.Setup(data, helpers)
			registryTokenAuthHTTPSRandom.Setup(data, helpers)
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			if registryTokenAuthHTTPSRandom != nil {
				registryTokenAuthHTTPSRandom.Cleanup(data, helpers)
			}
			if tokenServer != nil {
				tokenServer.Cleanup(data, helpers)
			}
		},
		SubTests: []*test.Case{
			{
				Description: "push-to-registry",
				Require:     require.Not(nerdtest.Docker),
				Setup: func(data test.Data, helpers test.Helpers) {
					targetRef := fmt.Sprintf("%s:%d/%s",
						registryTokenAuthHTTPSRandom.IP.String(), registryTokenAuthHTTPSRandom.Port, "test-list-push:v1")
					helpers.Ensure("pull", manifestRef)
					helpers.Ensure("tag", manifestRef, targetRef)
					helpers.Ensure("--hosts-dir", registryTokenAuthHTTPSRandom.HostsDir, "login", "-u", "admin", "-p", "badmin",
						fmt.Sprintf("%s:%d", registryTokenAuthHTTPSRandom.IP.String(), registryTokenAuthHTTPSRandom.Port))
					helpers.Ensure("push", "--hosts-dir", registryTokenAuthHTTPSRandom.HostsDir, targetRef)
					helpers.Ensure("rmi", targetRef)
					helpers.Ensure("manifest", "create", "--insecure", targetRef+"-success", targetRef)
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					targetRef := fmt.Sprintf("%s:%d/%s",
						registryTokenAuthHTTPSRandom.IP.String(), registryTokenAuthHTTPSRandom.Port, "test-list-push:v1")
					return helpers.Command("manifest", "push", "--insecure", targetRef+"-success")
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						ExitCode: 0,
						Output:   expect.Contains(data.Labels().Get("output")),
					}
				},
				Data: test.WithLabels(map[string]string{
					"output": expectedDigest,
				}),
			},
		},
	}
	testCase.Run(t)
}
