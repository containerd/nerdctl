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

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/registry"
)

func TestImagePullWithCosign(t *testing.T) {
	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)

	nerdtest.Setup()

	var reg *registry.Server

	testCase := &test.Case{
		Require: require.All(
			require.Linux,
			nerdtest.Build,
			require.Binary("cosign"),
			require.Not(nerdtest.Docker),
			nerdtest.Registry,
		),

		Env: map[string]string{
			"COSIGN_PASSWORD": "1",
		},

		Setup: func(data test.Data, helpers test.Helpers) {
			data.Temp().Save(dockerfile, "Dockerfile")
			pri, pub := nerdtest.GenerateCosignKeyPair(data, helpers, "1")
			reg = nerdtest.RegistryWithNoAuth(data, helpers, 0, false)
			reg.Setup(data, helpers)
			testImageRef := fmt.Sprintf("%s:%d/%s", "127.0.0.1", reg.Port, data.Identifier())
			buildCtx := data.Temp().Path()

			helpers.Ensure("build", "-t", testImageRef+":one", buildCtx)
			helpers.Ensure("build", "-t", testImageRef+":two", buildCtx)
			helpers.Ensure("push", "--sign=cosign", "--cosign-key="+pri, testImageRef+":one")
			helpers.Ensure("push", "--sign=cosign", "--cosign-key="+pri, testImageRef+":two")

			data.Labels().Set("public_key", pub)
			data.Labels().Set("image_ref", testImageRef)
		},

		Cleanup: func(data test.Data, helpers test.Helpers) {
			if reg != nil {
				reg.Cleanup(data, helpers)
				testImageRef := data.Labels().Get("image_ref")
				helpers.Anyhow("rmi", "-f", testImageRef+":one")
				helpers.Anyhow("rmi", "-f", testImageRef+":two")
			}
		},

		SubTests: []*test.Case{
			{
				Description: "Pull with the correct key",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command(
						"pull", "--quiet", "--verify=cosign",
						"--cosign-key="+data.Labels().Get("public_key"),
						data.Labels().Get("image_ref")+":one")
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "Pull with unrelated key",
				Env: map[string]string{
					"COSIGN_PASSWORD": "2",
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					_, pub := nerdtest.GenerateCosignKeyPair(data, helpers, "2")
					return helpers.Command("pull", "--quiet", "--verify=cosign", "--cosign-key="+pub, data.Labels().Get("image_ref")+":two")
				},
				Expected: test.Expects(12, nil, nil),
			},
		},
	}

	testCase.Run(t)
}

func TestImagePullPlainHttpWithDefaultPort(t *testing.T) {
	nerdtest.Setup()

	var reg *registry.Server
	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)

	testCase := &test.Case{
		Require: require.All(
			require.Linux,
			require.Not(nerdtest.Docker),
			nerdtest.Build,
			nerdtest.Registry,
		),

		Setup: func(data test.Data, helpers test.Helpers) {
			data.Temp().Save(dockerfile, "Dockerfile")
			reg = nerdtest.RegistryWithNoAuth(data, helpers, 80, false)
			reg.Setup(data, helpers)
			testImageRef := fmt.Sprintf("%s/%s:%s",
				reg.IP.String(), data.Identifier(), strings.Split(testutil.CommonImage, ":")[1])
			buildCtx := data.Temp().Path()

			helpers.Ensure("build", "-t", testImageRef, buildCtx)
			helpers.Ensure("--insecure-registry", "push", testImageRef)
			helpers.Ensure("rmi", "-f", testImageRef)

			data.Labels().Set("image_ref", testImageRef)
		},

		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("--insecure-registry", "pull", data.Labels().Get("image_ref"))
		},

		Expected: test.Expects(0, nil, nil),

		Cleanup: func(data test.Data, helpers test.Helpers) {
			if reg != nil {
				reg.Cleanup(data, helpers)
				helpers.Anyhow("rmi", "-f", data.Labels().Get("image_ref"))
			}
		},
	}

	testCase.Run(t)
}

func TestImagePullSoci(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Require: require.All(
			require.Linux,
			require.Not(nerdtest.Docker),
			nerdtest.Soci,
		),

		// NOTE: these tests cannot be run in parallel, as they depend on the output of host `mount`
		// They also feel prone to raciness...
		NoParallel: true,

		SubTests: []*test.Case{
			{
				Description: "Run without specifying SOCI index",
				NoParallel:  true,
				Data: test.WithLabels(map[string]string{
					"remoteSnapshotsExpectedCount": "11",
					"sociIndexDigest":              "",
				}),
				Setup: func(data test.Data, helpers test.Helpers) {
					cmd := helpers.Custom("mount")
					cmd.Run(&test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							data.Labels().Set("remoteSnapshotsInitialCount", strconv.Itoa(strings.Count(stdout, "fuse.rawBridge")))
						},
					})
					helpers.Ensure("--snapshotter=soci", "pull", testutil.FfmpegSociImage)
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", testutil.FfmpegSociImage)
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Custom("mount")
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, _ string, t *testing.T) {
							remoteSnapshotsInitialCount, _ := strconv.Atoi(data.Labels().Get("remoteSnapshotsInitialCount"))
							remoteSnapshotsActualCount := strings.Count(stdout, "fuse.rawBridge")
							assert.Equal(t,
								data.Labels().Get("remoteSnapshotsExpectedCount"),
								strconv.Itoa(remoteSnapshotsActualCount-remoteSnapshotsInitialCount),
								"expected remote snapshot count to match",
							)
						},
					}
				},
			},
			{
				Description: "Run with bad SOCI index",
				NoParallel:  true,
				Data: test.WithLabels(map[string]string{
					"remoteSnapshotsExpectedCount": "11",
					"sociIndexDigest":              "sha256:thisisabadindex0000000000000000000000000000000000000000000000000",
				}),
				Setup: func(data test.Data, helpers test.Helpers) {
					cmd := helpers.Custom("mount")
					cmd.Run(&test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							data.Labels().Set("remoteSnapshotsInitialCount", strconv.Itoa(strings.Count(stdout, "fuse.rawBridge")))
						},
					})
					helpers.Ensure("--snapshotter=soci", "pull", testutil.FfmpegSociImage)
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", testutil.FfmpegSociImage)
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Custom("mount")
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							remoteSnapshotsInitialCount, _ := strconv.Atoi(data.Labels().Get("remoteSnapshotsInitialCount"))
							remoteSnapshotsActualCount := strings.Count(stdout, "fuse.rawBridge")
							assert.Equal(t,
								data.Labels().Get("remoteSnapshotsExpectedCount"),
								strconv.Itoa(remoteSnapshotsActualCount-remoteSnapshotsInitialCount),
								"expected remote snapshot count to match")
						},
					}
				},
			},
		},
	}

	testCase.Run(t)
}

func TestImagePullProcessOutput(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		SubTests: []*test.Case{
			{
				Description: "Pull Image - output should be in stdout",
				NoParallel:  true,
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", testutil.BusyboxImage)
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("pull", testutil.BusyboxImage)
				},
				Expected: test.Expects(0, nil, expect.Contains(testutil.BusyboxImage)),
			},
			{
				Description: "Run Container with image pull - output should be in stderr",
				NoParallel:  true,
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", "-f", testutil.BusyboxImage)
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", testutil.BusyboxImage)
				},
				Expected: test.Expects(0, nil, expect.DoesNotContain(testutil.BusyboxImage)),
			},
		},
	}

	testCase.Run(t)
}
