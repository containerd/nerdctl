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
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	testhelpers "github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestImagePrune(t *testing.T) {
	testCase := nerdtest.Setup()

	// Cannot use a custom namespace with buildkitd right now, so, no parallel it is
	testCase.NoParallel = true
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		// We need to delete everything here for prune to make any sense
		base := testutil.NewBase(t)
		testhelpers.RmiAll(base)
	}
	testCase.SubTests = []*test.Case{
		{
			Description: "without all",
			NoParallel:  true,
			Require: test.Require(
				// This never worked with Docker - the only reason we ever got <none> was side effects from other tests
				// See inline comments.
				test.Not(nerdtest.Docker),
				nerdtest.Build,
			),
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rmi", data.Identifier())
			},
			Setup: func(data test.Data, helpers test.Helpers) {
				dockerfile := fmt.Sprintf(`FROM %s
				CMD ["echo", "nerdctl-test-image-prune"]
					`, testutil.CommonImage)

				buildCtx := testhelpers.CreateBuildContext(t, dockerfile)
				helpers.Ensure("build", buildCtx)
				// After we rebuild with tag, docker will no longer show the <none> version from above
				// Swapping order does not change anything.
				helpers.Ensure("build", "-t", data.Identifier(), buildCtx)
				imgList := helpers.Capture("images")
				assert.Assert(t, strings.Contains(imgList, "<none>"), "Missing <none>")
				assert.Assert(t, strings.Contains(imgList, data.Identifier()), "Missing "+data.Identifier())
			},
			Command: test.RunCommand("image", "prune", "--force"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: test.All(
						func(stdout string, info string, t *testing.T) {
							assert.Assert(t, !strings.Contains(stdout, data.Identifier()), info)
						},
						func(stdout string, info string, t *testing.T) {
							imgList := helpers.Capture("images")
							assert.Assert(t, !strings.Contains(imgList, "<none>"), imgList)
							assert.Assert(t, strings.Contains(imgList, data.Identifier()), info)
						},
					),
				}
			},
		},
		{
			Description: "with all",
			Require: test.Require(
				// Same as above
				test.Not(nerdtest.Docker),
				nerdtest.Build,
			),
			// Cannot use a custom namespace with buildkitd right now, so, no parallel it is
			NoParallel: true,
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rmi", data.Identifier())
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Setup: func(data test.Data, helpers test.Helpers) {
				dockerfile := fmt.Sprintf(`FROM %s
				CMD ["echo", "nerdctl-test-image-prune"]
					`, testutil.CommonImage)

				buildCtx := testhelpers.CreateBuildContext(t, dockerfile)
				helpers.Ensure("build", buildCtx)
				helpers.Ensure("build", "-t", data.Identifier(), buildCtx)
				imgList := helpers.Capture("images")
				assert.Assert(t, strings.Contains(imgList, "<none>"), "Missing <none>")
				assert.Assert(t, strings.Contains(imgList, data.Identifier()), "Missing "+data.Identifier())
				helpers.Ensure("run", "--name", data.Identifier(), data.Identifier())
			},
			Command: test.RunCommand("image", "prune", "--force", "--all"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: test.All(
						func(stdout string, info string, t *testing.T) {
							assert.Assert(t, !strings.Contains(stdout, data.Identifier()), info)
						},
						func(stdout string, info string, t *testing.T) {
							imgList := helpers.Capture("images")
							assert.Assert(t, strings.Contains(imgList, data.Identifier()), info)
							assert.Assert(t, !strings.Contains(imgList, "<none>"), imgList)
							helpers.Ensure("rm", "-f", data.Identifier())
							removed := helpers.Capture("image", "prune", "--force", "--all")
							assert.Assert(t, strings.Contains(removed, data.Identifier()), info)
							imgList = helpers.Capture("images")
							assert.Assert(t, !strings.Contains(imgList, data.Identifier()), info)
						},
					),
				}
			},
		},
		{
			Description: "with filter label",
			Require:     nerdtest.Build,
			// Cannot use a custom namespace with buildkitd right now, so, no parallel it is
			NoParallel: true,
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rmi", data.Identifier())
			},
			Setup: func(data test.Data, helpers test.Helpers) {
				dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-test-image-prune-filter-label"]
LABEL foo=bar
LABEL version=0.1`, testutil.CommonImage)
				buildCtx := testhelpers.CreateBuildContext(t, dockerfile)
				helpers.Ensure("build", "-t", data.Identifier(), buildCtx)
				imgList := helpers.Capture("images")
				assert.Assert(t, strings.Contains(imgList, data.Identifier()), "Missing "+data.Identifier())
			},
			Command: test.RunCommand("image", "prune", "--force", "--all", "--filter", "label=foo=baz"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: test.All(
						func(stdout string, info string, t *testing.T) {
							assert.Assert(t, !strings.Contains(stdout, data.Identifier()), info)
						},
						func(stdout string, info string, t *testing.T) {
							imgList := helpers.Capture("images")
							assert.Assert(t, strings.Contains(imgList, data.Identifier()), info)
						},
						func(stdout string, info string, t *testing.T) {
							prune := helpers.Capture("image", "prune", "--force", "--all", "--filter", "label=foo=bar")
							assert.Assert(t, strings.Contains(prune, data.Identifier()), info)
							imgList := helpers.Capture("images")
							assert.Assert(t, !strings.Contains(imgList, data.Identifier()), info)
						},
					),
				}
			},
		},
		{
			Description: "with until",
			Require:     nerdtest.Build,
			// Cannot use a custom namespace with buildkitd right now, so, no parallel it is
			NoParallel: true,
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rmi", data.Identifier())
			},
			Setup: func(data test.Data, helpers test.Helpers) {
				dockerfile := fmt.Sprintf(`FROM %s
RUN echo "Anything, so that we create actual content for docker to set the current time for CreatedAt"
CMD ["echo", "nerdctl-test-image-prune-until"]`, testutil.CommonImage)
				buildCtx := testhelpers.CreateBuildContext(t, dockerfile)
				helpers.Ensure("build", "-t", data.Identifier(), buildCtx)
				imgList := helpers.Capture("images")
				assert.Assert(t, strings.Contains(imgList, data.Identifier()), "Missing "+data.Identifier())
				data.Set("imageID", data.Identifier())
			},
			Command: test.RunCommand("image", "prune", "--force", "--all", "--filter", "until=12h"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: test.All(
						test.DoesNotContain(data.Get("imageID")),
						func(stdout string, info string, t *testing.T) {
							imgList := helpers.Capture("images")
							assert.Assert(t, strings.Contains(imgList, data.Get("imageID")), info)
						},
					),
				}
			},
			SubTests: []*test.Case{
				{
					Description: "Wait and remove until=10ms",
					NoParallel:  true,
					Setup: func(data test.Data, helpers test.Helpers) {
						time.Sleep(1 * time.Second)
					},
					Command: test.RunCommand("image", "prune", "--force", "--all", "--filter", "until=10ms"),
					Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
						return &test.Expected{
							Output: test.All(
								test.Contains(data.Get("imageID")),
								func(stdout string, info string, t *testing.T) {
									imgList := helpers.Capture("images")
									assert.Assert(t, !strings.Contains(imgList, data.Get("imageID")), imgList, info)
								},
							),
						}
					},
				},
			},
		},
	}

	testCase.Run(t)
}
