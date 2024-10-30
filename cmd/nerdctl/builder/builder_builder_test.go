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
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestBuilder(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		NoParallel: true,
		Require: test.Require(
			nerdtest.Build,
			test.Not(test.Windows),
		),
		SubTests: []*test.Case{
			{
				Description: "PruneForce",
				NoParallel:  true,
				Setup: func(data test.Data, helpers test.Helpers) {
					dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-test-builder-prune"]`, testutil.CommonImage)
					buildCtx := data.TempDir()
					err := os.WriteFile(filepath.Join(buildCtx, "Dockerfile"), []byte(dockerfile), 0o600)
					assert.NilError(helpers.T(), err)
					helpers.Ensure("build", buildCtx)
				},
				Command:  test.Command("builder", "prune", "--force"),
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "PruneForceAll",
				NoParallel:  true,
				Setup: func(data test.Data, helpers test.Helpers) {
					dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-test-builder-prune"]`, testutil.CommonImage)
					buildCtx := data.TempDir()
					err := os.WriteFile(filepath.Join(buildCtx, "Dockerfile"), []byte(dockerfile), 0o600)
					assert.NilError(helpers.T(), err)
					helpers.Ensure("build", buildCtx)
				},
				Command:  test.Command("builder", "prune", "--force", "--all"),
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "Debug",
				NoParallel:  true,
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-builder-debug-test-string"]`, testutil.CommonImage)
					buildCtx := data.TempDir()
					err := os.WriteFile(filepath.Join(buildCtx, "Dockerfile"), []byte(dockerfile), 0o600)
					assert.NilError(helpers.T(), err)
					cmd := helpers.Command("builder", "debug", buildCtx)
					cmd.WithStdin(bytes.NewReader([]byte("c\n")))
					return cmd
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "WithPull",
				Setup: func(data test.Data, helpers test.Helpers) {
					// FIXME: this test should be rewritten to dynamically retrieve the ids, and use images
					// available on all platforms
					oldImage := testutil.BusyboxImage
					oldImageSha := "7b3ccabffc97de872a30dfd234fd972a66d247c8cfc69b0550f276481852627c"
					newImage := testutil.AlpineImage
					newImageSha := "ec14c7992a97fc11425907e908340c6c3d6ff602f5f13d899e6b7027c9b4133a"

					helpers.Ensure("pull", "--quiet", oldImage)
					helpers.Ensure("tag", oldImage, newImage)

					dockerfile := fmt.Sprintf(`FROM %s`, newImage)
					buildCtx := data.TempDir()
					err := os.WriteFile(filepath.Join(buildCtx, "Dockerfile"), []byte(dockerfile), 0o600)
					assert.NilError(helpers.T(), err)

					data.Set("buildCtx", buildCtx)
					data.Set("oldImageSha", oldImageSha)
					data.Set("newImageSha", newImageSha)
				},
				SubTests: []*test.Case{
					{
						Description: "pull false",
						NoParallel:  true,
						Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
							return helpers.Command("build", data.Get("buildCtx"), "--pull=false")
						},
						Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
							return &test.Expected{
								Errors: []error{errors.New(data.Get("oldImageSha"))},
							}
						},
					},
					{
						Description: "pull true",
						NoParallel:  true,
						Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
							return helpers.Command("build", data.Get("buildCtx"), "--pull=true")
						},
						Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
							return &test.Expected{
								Errors: []error{errors.New(data.Get("newImageSha"))},
							}
						},
					},
					{
						Description: "no pull",
						NoParallel:  true,
						Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
							return helpers.Command("build", data.Get("buildCtx"))
						},
						Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
							return &test.Expected{
								Errors: []error{errors.New(data.Get("newImageSha"))},
							}
						},
					},
				},
			},
		},
	}

	testCase.Run(t)
}
