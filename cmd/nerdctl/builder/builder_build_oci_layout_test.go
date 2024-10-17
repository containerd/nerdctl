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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestBuildContextWithOCILayout(t *testing.T) {
	nerdtest.Setup()

	var dockerBuilderArgs []string

	testCase := &test.Case{
		Require: test.Require(
			nerdtest.Build,
			test.Not(test.Windows),
		),
		Cleanup: func(data test.Data, helpers test.Helpers) {
			if nerdtest.IsDocker() {
				helpers.Anyhow("buildx", "stop", data.Identifier("-container"))
				helpers.Anyhow("buildx", "rm", "--force", data.Identifier("-container"))
			}
			helpers.Anyhow("rmi", "-f", data.Identifier("-parent"))
			helpers.Anyhow("rmi", "-f", data.Identifier("-child"))
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			// Default docker driver does not support OCI exporter.
			// Reference: https://docs.docker.com/build/exporters/oci-docker/
			if nerdtest.IsDocker() {
				name := data.Identifier("-container")
				helpers.Ensure("buildx", "create", "--name", name, "--driver=docker-container")
				dockerBuilderArgs = []string{"buildx", "--builder", name}
			}

			dockerfile := fmt.Sprintf(`FROM %s
LABEL layer=oci-layout-parent
CMD ["echo", "test-nerdctl-build-context-oci-layout-parent"]`, testutil.CommonImage)

			buildCtx := data.TempDir()
			err := os.WriteFile(filepath.Join(buildCtx, "Dockerfile"), []byte(dockerfile), 0o600)
			assert.NilError(helpers.T(), err)

			tarPath := filepath.Join(buildCtx, "parent.tar")
			dest := filepath.Join(buildCtx, "parent")
			assert.NilError(helpers.T(), os.MkdirAll(dest, 0o700))
			helpers.Ensure("build", buildCtx, "--tag", data.Identifier("-parent"))
			helpers.Ensure("image", "save", "--output", tarPath, data.Identifier("-parent"))
			helpers.Custom("tar", "Cxf", dest, tarPath).Run(&test.Expected{})
		},

		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			dockerfile := `FROM parent
CMD ["echo", "test-nerdctl-build-context-oci-layout"]`

			buildCtx := data.TempDir()
			err := os.WriteFile(filepath.Join(buildCtx, "Dockerfile"), []byte(dockerfile), 0o600)
			assert.NilError(helpers.T(), err)

			var cmd test.TestableCommand
			if nerdtest.IsDocker() {
				cmd = helpers.Command(dockerBuilderArgs...)
			} else {
				cmd = helpers.Command()
			}
			cmd.WithArgs("build", buildCtx, fmt.Sprintf("--build-context=parent=oci-layout://%s", filepath.Join(buildCtx, "parent")), "--tag", data.Identifier("-child"))
			if nerdtest.IsDocker() {
				// Need to load the container image from the builder to be able to run it.
				cmd.WithArgs("--load")
			}
			return cmd
		},
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			return &test.Expected{
				Output: func(stdout string, info string, t *testing.T) {
					assert.Assert(t, strings.Contains(helpers.Capture("run", "--rm", data.Identifier("-child")), "test-nerdctl-build-context-oci-layout"), info)
				},
			}
		},
	}

	testCase.Run(t)
}
