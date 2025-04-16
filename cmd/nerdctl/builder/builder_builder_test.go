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
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestBuilder(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		NoParallel: true,
		Require: require.All(
			nerdtest.Build,
			require.Not(require.Windows),
		),
		SubTests: []*test.Case{
			{
				Description: "PruneForce",
				NoParallel:  true,
				Setup: func(data test.Data, helpers test.Helpers) {
					dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-test-builder-prune"]`, testutil.CommonImage)
					data.Temp().Save(dockerfile, "Dockerfile")
					helpers.Ensure("build", data.Temp().Path())
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
					data.Temp().Save(dockerfile, "Dockerfile")
					helpers.Ensure("build", data.Temp().Path())
				},
				Command:  test.Command("builder", "prune", "--force", "--all"),
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "Debug",
				// `nerdctl builder debug` is currently incompatible with `docker buildx debug`.
				Require:    require.All(require.Not(nerdtest.Docker)),
				NoParallel: true,
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-builder-debug-test-string"]`, testutil.CommonImage)
					data.Temp().Save(dockerfile, "Dockerfile")
					cmd := helpers.Command("builder", "debug", data.Temp().Path())
					cmd.Feed(strings.NewReader("c\n"))
					return cmd
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "WithPull",
				NoParallel:  true,
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
					data.Temp().Save(dockerfile, "Dockerfile")
					data.Labels().Set("oldImageSha", oldImageSha)
					data.Labels().Set("newImageSha", newImageSha)
					data.Labels().Set("base", data.Temp().Dir())
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rmi", testutil.AlpineImage)
				},
				SubTests: []*test.Case{
					{
						Description: "pull false",
						NoParallel:  true,
						Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
							return helpers.Command("build", data.Labels().Get("base"), "--pull=false")
						},
						Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
							return &test.Expected{
								Errors: []error{errors.New(data.Labels().Get("oldImageSha"))},
							}
						},
					},
					{
						Description: "pull true",
						NoParallel:  true,
						Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
							return helpers.Command("build", data.Labels().Get("base"), "--pull=true")
						},
						Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
							return &test.Expected{
								Errors: []error{errors.New(data.Labels().Get("newImageSha"))},
							}
						},
					},
					{
						Description: "no pull",
						NoParallel:  true,
						Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
							return helpers.Command("build", data.Labels().Get("base"))
						},
						Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
							return &test.Expected{
								Errors: []error{errors.New(data.Labels().Get("newImageSha"))},
							}
						},
					},
				},
			},
		},
	}

	testCase.Run(t)
}
