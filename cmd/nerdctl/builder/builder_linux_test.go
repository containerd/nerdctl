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
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"gotest.tools/v3/assert"

	testhelpers "github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestBuilder(t *testing.T) {
	nerdtest.Setup()

	// FIXME: this is a dirty hack to pass a function from Setup to Cleanup, which is not currently possible
	var bkGC func()

	testCase := &test.Case{
		Require: nerdtest.Build,
		SubTests: []*test.Case{
			{
				Description: "PruneForce",
				Setup: func(data test.Data, helpers test.Helpers) {
					dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-test-builder-prune"]`, testutil.CommonImage)
					buildCtx := testhelpers.CreateBuildContext(t, dockerfile)
					helpers.Ensure("build", buildCtx)
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("builder", "prune", "--all", "--force")
				},
				Command:  test.RunCommand("builder", "prune", "--force"),
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "PruneForceAll",
				Setup: func(data test.Data, helpers test.Helpers) {
					dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-test-builder-prune"]`, testutil.CommonImage)
					buildCtx := testhelpers.CreateBuildContext(t, dockerfile)
					helpers.Ensure("build", buildCtx)
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("builder", "prune", "--all", "--force")
				},
				Command:  test.RunCommand("builder", "prune", "--force", "--all"),
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "Debug",
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("builder", "prune", "--all", "--force")
				},
				Command: func(data test.Data, helpers test.Helpers) test.Command {
					dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-builder-debug-test-string"]`, testutil.CommonImage)
					buildCtx := testhelpers.CreateBuildContext(t, dockerfile)
					cmd := helpers.Command("builder", "debug", buildCtx)
					cmd.WithStdin(bytes.NewReader([]byte("c\n")))
					return cmd
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "WithPull",
				Require:     nerdtest.RootFul,
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("builder", "prune", "--all", "--force")
					if bkGC != nil {
						bkGC()
					}
				},
				Setup: func(data test.Data, helpers test.Helpers) {
					buildkitConfig := fmt.Sprintf(`[worker.oci]
enabled = false

[worker.containerd]
enabled = true
namespace = "%s"`, testutil.Namespace)

					bkGC = useBuildkitConfig(t, buildkitConfig)
					oldImage := testutil.BusyboxImage
					oldImageSha := "141c253bc4c3fd0a201d32dc1f493bcf3fff003b6df416dea4f41046e0f37d47"
					newImage := testutil.AlpineImage

					helpers.Ensure("pull", oldImage)
					helpers.Ensure("tag", oldImage, newImage)

					dockerfile := fmt.Sprintf(`FROM %s`, newImage)
					buildCtx := testhelpers.CreateBuildContext(t, dockerfile)

					data.Set("buildCtx", buildCtx)
					data.Set("oldImageSha", oldImageSha)
				},
				SubTests: []*test.Case{
					{
						Command: func(data test.Data, helpers test.Helpers) test.Command {
							return helpers.Command("build", data.Get("buildCtx"), "--pull=false")
						},
						Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
							return &test.Expected{
								ExitCode: 1,
								Errors:   []error{errors.New(data.Get("oldImageSha"))},
							}
						},
					},
					{
						Command: func(data test.Data, helpers test.Helpers) test.Command {
							return helpers.Command("build", data.Get("buildCtx"), "--pull=true")
						},
						Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
							return &test.Expected{
								ExitCode: 0,
							}
						},
					},
					{
						Command: func(data test.Data, helpers test.Helpers) test.Command {
							return helpers.Command("build", data.Get("buildCtx"))
						},
						Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
							return &test.Expected{
								ExitCode: 0,
							}
						},
					},
				},
			},
		},
	}

	testCase.Run(t)
}

func useBuildkitConfig(t *testing.T, config string) (cleanup func()) {
	buildkitConfigPath := "/etc/buildkit/buildkitd.toml"

	currConfig, err := exec.Command("cat", buildkitConfigPath).Output()
	assert.NilError(t, err)

	os.WriteFile(buildkitConfigPath, []byte(config), 0644)
	_, err = exec.Command("systemctl", "restart", "buildkit").Output()
	assert.NilError(t, err)

	return func() {
		assert.NilError(t, os.WriteFile(buildkitConfigPath, currConfig, 0644))
		_, err = exec.Command("systemctl", "restart", "buildkit").Output()
		assert.NilError(t, err)
	}
}
