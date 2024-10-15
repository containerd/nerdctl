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
	"slices"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	testhelpers "github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/tabutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestImages(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Description: "TestImages",
		Require:     test.Not(nerdtest.Docker),
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("pull", "--quiet", testutil.CommonImage)
			helpers.Ensure("pull", "--quiet", testutil.NginxAlpineImage)
		},
		SubTests: []*test.Case{
			{
				Description: "No params",
				Command:     test.Command("images"),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							lines := strings.Split(strings.TrimSpace(stdout), "\n")
							assert.Assert(t, len(lines) >= 2, info)
							header := "REPOSITORY\tTAG\tIMAGE ID\tCREATED\tPLATFORM\tSIZE\tBLOB SIZE"
							if nerdtest.IsDocker() {
								header = "REPOSITORY\tTAG\tIMAGE ID\tCREATED\tSIZE"
							}
							tab := tabutil.NewReader(header)
							err := tab.ParseHeader(lines[0])
							assert.NilError(t, err, info)
							found := false
							for _, line := range lines[1:] {
								repo, _ := tab.ReadRow(line, "REPOSITORY")
								tag, _ := tab.ReadRow(line, "TAG")
								if repo+":"+tag == testutil.CommonImage {
									found = true
									break
								}
							}
							assert.Assert(t, found, info)
						},
					}
				},
			},
			{
				Description: "With names",
				Command:     test.Command("images", "--names", testutil.CommonImage),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: test.All(
							test.Contains(testutil.CommonImage),
							func(stdout string, info string, t *testing.T) {
								lines := strings.Split(strings.TrimSpace(stdout), "\n")
								assert.Assert(t, len(lines) >= 2, info)
								tab := tabutil.NewReader("NAME\tIMAGE ID\tCREATED\tPLATFORM\tSIZE\tBLOB SIZE")
								err := tab.ParseHeader(lines[0])
								assert.NilError(t, err, info)
								found := false
								for _, line := range lines[1:] {
									name, _ := tab.ReadRow(line, "NAME")
									if name == testutil.CommonImage {
										found = true
										break
									}
								}

								assert.Assert(t, found, info)
							},
						),
					}
				},
			},
			{
				Description: "CheckCreatedTime",
				Command:     test.Command("images", "--format", "'{{json .CreatedAt}}'"),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							lines := strings.Split(strings.TrimSpace(stdout), "\n")
							assert.Assert(t, len(lines) >= 2, info)
							createdTimes := lines
							slices.Reverse(createdTimes)
							assert.Assert(t, slices.IsSorted(createdTimes), info)
						},
					}
				},
			},
		},
	}

	testCase.Run(t)
}

func TestImagesFilter(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Description: "TestImagesFilter",
		Require:     nerdtest.Build,
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("pull", "--quiet", testutil.CommonImage)
			helpers.Ensure("tag", testutil.CommonImage, "taggedimage:one-fragment-one")
			helpers.Ensure("tag", testutil.CommonImage, "taggedimage:two-fragment-two")

			dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"] \n
LABEL foo=bar
LABEL version=0.1
RUN echo "actually creating a layer so that docker sets the createdAt time"
`, testutil.CommonImage)
			buildCtx := testhelpers.CreateBuildContext(t, dockerfile)
			data.Set("buildCtx", buildCtx)
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", "taggedimage:one-fragment-one")
			helpers.Anyhow("rmi", "-f", "taggedimage:two-fragment-two")
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			data.Set("builtImageID", data.Identifier())
			return helpers.Command("build", "-t", data.Identifier(), data.Get("buildCtx"))
		},
		Expected: test.Expects(0, nil, nil),
		SubTests: []*test.Case{
			{
				Description: "label=foo=bar",
				Command:     test.Command("images", "--filter", "label=foo=bar"),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: test.Contains(data.Get("builtImageID")),
					}
				},
			},
			{
				Description: "label=foo=bar1",
				Command:     test.Command("images", "--filter", "label=foo=bar1"),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: test.DoesNotContain(data.Get("builtImageID")),
					}
				},
			},
			{
				Description: "label=foo=bar label=version=0.1",
				Command:     test.Command("images", "--filter", "label=foo=bar", "--filter", "label=version=0.1"),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: test.Contains(data.Get("builtImageID")),
					}
				},
			},
			{
				Description: "label=foo=bar label=version=0.1",
				Command:     test.Command("images", "--filter", "label=foo=bar", "--filter", "label=version=0.2"),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: test.DoesNotContain(data.Get("builtImageID")),
					}
				},
			},
			{
				Description: "label=version",
				Require:     nerdtest.IsFlaky("https://github.com/containerd/nerdctl/issues/3512"),
				Command:     test.Command("images", "--filter", "label=version"),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: test.Contains(data.Get("builtImageID")),
					}
				},
			},
			{
				Description: "reference=ID*",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("images", "--filter", fmt.Sprintf("reference=%s*", data.Get("builtImageID")))
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: test.Contains(data.Get("builtImageID")),
					}
				},
			},
			{
				Description: "reference=tagged*:*fragment*",
				Command:     test.Command("images", "--filter", "reference=tagged*:*fragment*"),
				Expected: test.Expects(0, nil, test.All(
					test.Contains("one-"),
					test.Contains("two-"),
				)),
			},
			{
				Description: "before=ID:latest",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("images", "--filter", fmt.Sprintf("before=%s:latest", data.Get("builtImageID")))
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: test.All(
							test.Contains(testutil.ImageRepo(testutil.CommonImage)),
							test.DoesNotContain(data.Get("builtImageID")),
						),
					}
				},
			},
			{
				Description: "since=" + testutil.CommonImage,
				Command:     test.Command("images", "--filter", fmt.Sprintf("since=%s", testutil.CommonImage)),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: test.All(
							test.Contains(data.Get("builtImageID")),
							test.DoesNotContain(testutil.ImageRepo(testutil.CommonImage)),
						),
					}
				},
			},
			{
				Description: "since=" + testutil.CommonImage + " " + testutil.CommonImage,
				Command:     test.Command("images", "--filter", fmt.Sprintf("since=%s", testutil.CommonImage), testutil.CommonImage),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: test.All(
							test.DoesNotContain(data.Get("builtImageID")),
							test.DoesNotContain(testutil.ImageRepo(testutil.CommonImage)),
						),
					}
				},
			},
			{
				Description: "since=non-exists-image",
				Require:     nerdtest.NerdctlNeedsFixing("https://github.com/containerd/nerdctl/issues/3511"),
				Command:     test.Command("images", "--filter", "since=non-exists-image"),
				Expected:    test.Expects(-1, nil, nil),
			},
			{
				Description: "before=non-exists-image",
				Require:     nerdtest.NerdctlNeedsFixing("https://github.com/containerd/nerdctl/issues/3511"),
				Command:     test.Command("images", "--filter", "before=non-exists-image"),
				Expected:    test.Expects(-1, nil, nil),
			},
		},
	}

	testCase.Run(t)
}

func TestImagesFilterDangling(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Description: "TestImagesFilterDangling",
		// This test relies on a clean slate and the ability to GC everything
		NoParallel: true,
		Require:    nerdtest.Build,
		Setup: func(data test.Data, helpers test.Helpers) {
			dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-notag-string"]
	`, testutil.CommonImage)
			buildCtx := testhelpers.CreateBuildContext(t, dockerfile)
			data.Set("buildCtx", buildCtx)
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("container", "prune", "-f")
			helpers.Anyhow("image", "prune", "--all", "-f")
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("build", data.Get("buildCtx"))
		},
		Expected: test.Expects(0, nil, nil),
		SubTests: []*test.Case{
			{
				Description: "dangling",
				Command:     test.Command("images", "--filter", "dangling=true"),
				Expected:    test.Expects(0, nil, test.Contains("<none>")),
			},
			{
				Description: "not dangling",
				Command:     test.Command("images", "--filter", "dangling=false"),
				Expected:    test.Expects(0, nil, test.DoesNotContain("<none>")),
			},
		},
	}

	testCase.Run(t)
}
