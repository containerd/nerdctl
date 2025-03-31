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

package container

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestCommit(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.SubTests = []*test.Case{
		{
			Description: "with pause",
			Require:     nerdtest.CGroup,
			Cleanup: func(data test.Data, helpers test.Helpers) {
				identifier := data.Identifier()
				helpers.Anyhow("rm", "-f", identifier)
				helpers.Anyhow("rmi", "-f", identifier)
			},
			Setup: func(data test.Data, helpers test.Helpers) {
				identifier := data.Identifier()
				helpers.Ensure("run", "-d", "--name", identifier, testutil.CommonImage, "sleep", nerdtest.Infinity)
				helpers.Ensure("exec", identifier, "sh", "-euxc", `echo hello-test-commit > /foo`)
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				identifier := data.Identifier()
				helpers.Ensure(
					"commit",
					"-c", `CMD ["/foo"]`,
					"-c", `ENTRYPOINT ["cat"]`,
					"--pause=true",
					identifier, identifier)
				return helpers.Command("run", "--rm", identifier)
			},
			Expected: test.Expects(0, nil, expect.Equals("hello-test-commit\n")),
		},
		{
			Description: "no pause",
			Require:     require.Not(require.Windows),
			Cleanup: func(data test.Data, helpers test.Helpers) {
				identifier := data.Identifier()
				helpers.Anyhow("rm", "-f", identifier)
				helpers.Anyhow("rmi", "-f", identifier)
			},
			Setup: func(data test.Data, helpers test.Helpers) {
				identifier := data.Identifier()
				helpers.Ensure("run", "-d", "--name", identifier, testutil.CommonImage, "sleep", nerdtest.Infinity)
				nerdtest.EnsureContainerStarted(helpers, identifier)
				helpers.Ensure("exec", identifier, "sh", "-euxc", `echo hello-test-commit > /foo`)
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				identifier := data.Identifier()
				helpers.Ensure(
					"commit",
					"-c", `CMD ["/foo"]`,
					"-c", `ENTRYPOINT ["cat"]`,
					"--pause=false",
					identifier, identifier)
				return helpers.Command("run", "--rm", identifier)
			},
			Expected: test.Expects(0, nil, expect.Equals("hello-test-commit\n")),
		},
	}

	testCase.Run(t)
}

func TestZstdCommit(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.All(
		// FIXME: Docker  does not support compression
		require.Not(nerdtest.Docker),
		nerdtest.ContainerdVersion("2.0.0"),
		nerdtest.CGroup,
	)
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
		helpers.Anyhow("rmi", "-f", data.Identifier("image"))
	}
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		identifier := data.Identifier()
		helpers.Ensure("run", "-d", "--name", identifier, testutil.CommonImage, "sleep", nerdtest.Infinity)
		nerdtest.EnsureContainerStarted(helpers, identifier)
		helpers.Ensure("exec", identifier, "sh", "-euxc", `echo hello-test-commit > /foo`)
		helpers.Ensure("commit", identifier, data.Identifier("image"), "--compression=zstd")
		data.Labels().Set("image", data.Identifier("image"))
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "verify zstd has been used",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("image", "inspect", "--mode=native", data.Labels().Get("image"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.JSON([]native.Image{}, func(images []native.Image, s string, t tig.T) {
						assert.Equal(t, len(images), 1)
						assert.Equal(helpers.T(), images[0].Manifest.Layers[len(images[0].Manifest.Layers)-1].MediaType, "application/vnd.docker.image.rootfs.diff.tar.zstd")
					}),
				}
			},
		},
		{
			Description: "verify the image is working",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", data.Labels().Get("image"), "sh", "-c", "--", "cat /foo")
			},
			Expected: test.Expects(0, nil, expect.Equals("hello-test-commit\n")),
		},
	}

	testCase.Run(t)
}
