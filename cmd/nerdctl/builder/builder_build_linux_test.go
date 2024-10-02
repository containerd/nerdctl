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

package builder

import (
	"fmt"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	testhelpers "github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestBuildContextWithOCILayout(t *testing.T) {
	nerdtest.Setup()

	testutil.RequiresBuild(t)
	testutil.RegisterBuildCacheCleanup(t)
	var dockerBuilderArgs []string
	if testutil.IsDocker() {
		// Default docker driver does not support OCI exporter.
		// Reference: https://docs.docker.com/build/exporters/oci-docker/
		builderName := testutil.SetupDockerContainerBuilder(t)
		dockerBuilderArgs = []string{"buildx", "--builder", builderName}
	}

	testCase := &test.Case{
		Description: "Build context OCI layout",
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", fmt.Sprintf("%s-parent", data.Identifier()))
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			dockerfile := fmt.Sprintf(`FROM %s
LABEL layer=oci-layout-parent
CMD ["echo", "test-nerdctl-build-context-oci-layout-parent"]`, testutil.CommonImage)

			// FIXME: replace with a generic file creation helper - search for all occurrences of temp file creation
			buildCtx := testhelpers.CreateBuildContext(t, dockerfile)
			tarPath := fmt.Sprintf("%s/parent.tar", buildCtx)

			helpers.Ensure("build", buildCtx, "--tag", fmt.Sprintf("%s-parent", data.Identifier()))
			helpers.Ensure("image", "save", "--output", tarPath, fmt.Sprintf("%s-parent", data.Identifier()))
			helpers.CustomCommand("tar", "Cxf", data.TempDir(), tarPath).Run(&test.Expected{})
		},

		Command: func(data test.Data, helpers test.Helpers) test.Command {
			dockerfile := `FROM parent
CMD ["echo", "test-nerdctl-build-context-oci-layout"]`
			buildCtx := testhelpers.CreateBuildContext(t, dockerfile)
			var cmd test.Command
			if testutil.IsDocker() {
				cmd = helpers.Command(dockerBuilderArgs...)
			} else {
				cmd = helpers.Command()
			}
			cmd.WithArgs("build", buildCtx, fmt.Sprintf("--build-context=parent=oci-layout://%s", data.TempDir()), "--tag", data.Identifier())
			if testutil.IsDocker() {
				// Need to load the container image from the builder to be able to run it.
				cmd.WithArgs("--load")
			}
			return cmd
		},
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			return &test.Expected{
				Output: func(stdout string, info string, t *testing.T) {
					assert.Assert(t, strings.Contains(helpers.Capture("run", "--rm", data.Identifier()), "test-nerdctl-build-context-oci-layout"), info)
				},
			}
		},
	}

	testCase.Run(t)
}
