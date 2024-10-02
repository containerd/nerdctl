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
	"strconv"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	testhelpers "github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
	"github.com/containerd/nerdctl/v2/pkg/testutil/testregistry"
)

func TestImagePullWithCosign(t *testing.T) {
	nerdtest.Setup()

	var registry *testregistry.RegistryServer
	var keyPair *testhelpers.CosignKeyPair

	testCase := &test.Case{
		Description: "TestImagePullWithCosign",
		Require: test.Require(
			test.Linux,
			nerdtest.Build,
			test.Binary("cosign"),
			test.Not(nerdtest.Docker),
		),
		Env: map[string]string{
			"COSIGN_PASSWORD": "1",
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			keyPair = testhelpers.NewCosignKeyPair(t, "cosign-key-pair", "1")
			base := testutil.NewBase(t)
			registry = testregistry.NewWithNoAuth(base, 80, false)
			testImageRef := fmt.Sprintf("%s/%s", "127.0.0.1", data.Identifier())
			dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)

			buildCtx := testhelpers.CreateBuildContext(t, dockerfile)
			helpers.Ensure("build", "-t", testImageRef, buildCtx)
			helpers.Ensure("push", "--sign=cosign", "--cosign-key="+keyPair.PrivateKey, testImageRef+":one")
			helpers.Ensure("push", "--sign=cosign", "--cosign-key="+keyPair.PrivateKey, testImageRef+":two")
			helpers.Ensure("rmi", "-f", testImageRef)
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			if keyPair != nil {
				keyPair.Cleanup()
			}
			if registry != nil {
				registry.Cleanup(nil)
				testImageRef := fmt.Sprintf("%s/%s", "127.0.0.1", data.Identifier())
				helpers.Anyhow("rmi", "-f", testImageRef)
			}
		},
		SubTests: []*test.Case{
			{
				Description: "Pull with the correct key",
				Command: func(data test.Data, helpers test.Helpers) test.Command {
					testImageRef := fmt.Sprintf("%s/%s", "127.0.0.1", data.Identifier())
					return helpers.Command("pull", "--verify=cosign", "--cosign-key="+keyPair.PublicKey, testImageRef+":one")
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "Pull with unrelated key",
				Env: map[string]string{
					"COSIGN_PASSWORD": "2",
				},
				Command: func(data test.Data, helpers test.Helpers) test.Command {
					newKeyPair := testhelpers.NewCosignKeyPair(t, "cosign-key-pair-test", "2")
					testImageRef := fmt.Sprintf("%s/%s", "127.0.0.1", data.Identifier())
					return helpers.Command("pull", "--verify=cosign", "--cosign-key="+newKeyPair.PublicKey, testImageRef+":two")
				},
				Expected: test.Expects(1, nil, nil),
			},
		},
	}

	testCase.Run(t)
}

func TestImagePullPlainHttpWithDefaultPort(t *testing.T) {
	nerdtest.Setup()

	var registry *testregistry.RegistryServer

	testCase := &test.Case{
		Description: "TestImagePullPlainHttpWithDefaultPort",
		Require: test.Require(
			test.Linux,
			test.Not(nerdtest.Docker),
			nerdtest.Build,
		),
		Setup: func(data test.Data, helpers test.Helpers) {
			base := testutil.NewBase(t)
			registry = testregistry.NewWithNoAuth(base, 80, false)
			testImageRef := fmt.Sprintf("%s/%s:%s",
				registry.IP.String(), data.Identifier(), strings.Split(testutil.CommonImage, ":")[1])
			dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)

			buildCtx := testhelpers.CreateBuildContext(t, dockerfile)
			helpers.Ensure("build", "-t", testImageRef, buildCtx)
			helpers.Ensure("--insecure-registry", "push", testImageRef)
			helpers.Ensure("rmi", "-f", testImageRef)
		},
		Command: func(data test.Data, helpers test.Helpers) test.Command {
			testImageRef := fmt.Sprintf("%s/%s:%s",
				registry.IP.String(), data.Identifier(), strings.Split(testutil.CommonImage, ":")[1])
			return helpers.Command("--insecure-registry", "pull", testImageRef)
		},
		Expected: test.Expects(0, nil, nil),
		Cleanup: func(data test.Data, helpers test.Helpers) {
			if registry != nil {
				registry.Cleanup(nil)
				testImageRef := fmt.Sprintf("%s/%s:%s",
					registry.IP.String(), data.Identifier(), strings.Split(testutil.CommonImage, ":")[1])
				helpers.Anyhow("rmi", "-f", testImageRef)
			}
		},
	}

	testCase.Run(t)
}

func TestImagePullSoci(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Description: "TestImagePullSoci",
		Require: test.Require(
			test.Linux,
			test.Not(nerdtest.Docker),
			nerdtest.Soci,
		),

		// NOTE: these tests cannot be run in parallel, as they depend on the output of host `mount`
		// They also feel prone to raciness...
		SubTests: []*test.Case{
			{
				Description: "Run without specifying SOCI index",
				NoParallel:  true,
				Data: test.
					WithData("remoteSnapshotsExpectedCount", "11").
					Set("sociIndexDigest", ""),
				Setup: func(data test.Data, helpers test.Helpers) {
					cmd := helpers.CustomCommand("mount")
					cmd.Run(&test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							data.Set("remoteSnapshotsInitialCount", strconv.Itoa(strings.Count(stdout, "fuse.rawBridge")))
						},
					})
					helpers.Ensure("--snapshotter=soci", "pull", testutil.FfmpegSociImage)
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", testutil.FfmpegSociImage)
				},
				Command: func(data test.Data, helpers test.Helpers) test.Command {
					return helpers.CustomCommand("mount")
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							remoteSnapshotsInitialCount, _ := strconv.Atoi(data.Get("remoteSnapshotsInitialCount"))
							remoteSnapshotsActualCount := strings.Count(stdout, "fuse.rawBridge")
							assert.Equal(t,
								data.Get("remoteSnapshotsExpectedCount"),
								strconv.Itoa(remoteSnapshotsActualCount-remoteSnapshotsInitialCount),
								info)
						},
					}
				},
			},
			{
				Description: "Run with bad SOCI index",
				NoParallel:  true,
				Data: test.
					WithData("remoteSnapshotsExpectedCount", "11").
					Set("sociIndexDigest", "sha256:thisisabadindex0000000000000000000000000000000000000000000000000"),
				Setup: func(data test.Data, helpers test.Helpers) {
					cmd := helpers.CustomCommand("mount")
					cmd.Run(&test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							data.Set("remoteSnapshotsInitialCount", strconv.Itoa(strings.Count(stdout, "fuse.rawBridge")))
						},
					})
					helpers.Ensure("--snapshotter=soci", "pull", testutil.FfmpegSociImage)
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", testutil.FfmpegSociImage)
				},
				Command: func(data test.Data, helpers test.Helpers) test.Command {
					return helpers.CustomCommand("mount")
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							remoteSnapshotsInitialCount, _ := strconv.Atoi(data.Get("remoteSnapshotsInitialCount"))
							remoteSnapshotsActualCount := strings.Count(stdout, "fuse.rawBridge")
							assert.Equal(t,
								data.Get("remoteSnapshotsExpectedCount"),
								strconv.Itoa(remoteSnapshotsActualCount-remoteSnapshotsInitialCount),
								info)
						},
					}
				},
			},
		},
	}

	testCase.Run(t)
}
