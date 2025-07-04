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
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
	"github.com/containerd/nerdctl/v2/pkg/tabutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestImages(t *testing.T) {
	nerdtest.Setup()

	commonImage, _ := referenceutil.Parse(testutil.CommonImage)

	testCase := &test.Case{
		Require: require.Not(nerdtest.Docker),
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("pull", "--quiet", commonImage.String())
			helpers.Ensure("pull", "--quiet", testutil.NginxAlpineImage)
		},
		SubTests: []*test.Case{
			{
				Description: "No params",
				Command:     test.Command("images"),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, t tig.T) {
							lines := strings.Split(strings.TrimSpace(stdout), "\n")
							assert.Assert(t, len(lines) >= 2, "there should be at least two lines\n")
							header := "REPOSITORY\tTAG\tIMAGE ID\tCREATED\tPLATFORM\tSIZE\tBLOB SIZE"
							if nerdtest.IsDocker() {
								header = "REPOSITORY\tTAG\tIMAGE ID\tCREATED\tSIZE"
							}
							tab := tabutil.NewReader(header)
							err := tab.ParseHeader(lines[0])
							assert.NilError(t, err, "ParseHeader should not fail\n")
							found := false
							for _, line := range lines[1:] {
								repo, _ := tab.ReadRow(line, "REPOSITORY")
								tag, _ := tab.ReadRow(line, "TAG")
								if repo+":"+tag == commonImage.FamiliarName()+":"+commonImage.Tag {
									found = true
									break
								}
							}
							assert.Assert(t, found, "we should have found an image\n")
						},
					}
				},
			},
			{
				Description: "With names",
				Command:     test.Command("images", "--names", commonImage.String()),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: expect.All(
							expect.Contains(commonImage.String()),
							func(stdout string, t tig.T) {
								lines := strings.Split(strings.TrimSpace(stdout), "\n")
								assert.Assert(t, len(lines) >= 2, "there should be at least two lines\n")
								tab := tabutil.NewReader("NAME\tIMAGE ID\tCREATED\tPLATFORM\tSIZE\tBLOB SIZE")
								err := tab.ParseHeader(lines[0])
								assert.NilError(t, err, "ParseHeader should not fail\n")
								found := false
								for _, line := range lines[1:] {
									name, _ := tab.ReadRow(line, "NAME")
									if name == commonImage.String() {
										found = true
										break
									}
								}

								assert.Assert(t, found, "we should have found an image\n")
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
						Output: func(stdout string, t tig.T) {
							lines := strings.Split(strings.TrimSpace(stdout), "\n")
							assert.Assert(t, len(lines) >= 2, "there should be at least two lines\n")
							createdTimes := lines
							slices.Reverse(createdTimes)
							assert.Assert(t, slices.IsSorted(createdTimes), "created times should be sorted\n")
						},
					}
				},
			},
		},
	}

	if runtime.GOOS == "windows" {
		testCase.Require = require.All(
			testCase.Require,
			nerdtest.IsFlaky("https://github.com/containerd/nerdctl/issues/3524"),
		)
	}

	testCase.Run(t)
}

func TestImagesFilter(t *testing.T) {
	nerdtest.Setup()

	commonImage, _ := referenceutil.Parse(testutil.CommonImage)

	testCase := &test.Case{
		Require: nerdtest.Build,
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("pull", "--quiet", commonImage.String())
			helpers.Ensure("tag", commonImage.String(), "taggedimage:one-fragment-one")
			helpers.Ensure("tag", commonImage.String(), "taggedimage:two-fragment-two")

			dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"] \n
LABEL foo=bar
LABEL version=0.1
RUN echo "actually creating a layer so that docker sets the createdAt time"
`, commonImage.String())
			buildCtx := data.Temp().Path()
			data.Temp().Save(dockerfile, "Dockerfile")
			data.Labels().Set("buildCtx", buildCtx)
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", "taggedimage:one-fragment-one")
			helpers.Anyhow("rmi", "-f", "taggedimage:two-fragment-two")
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			data.Labels().Set("builtImageID", data.Identifier())
			return helpers.Command("build", "-t", data.Identifier(), data.Labels().Get("buildCtx"))
		},
		Expected: test.Expects(0, nil, nil),
		SubTests: []*test.Case{
			{
				Description: "label=foo=bar",
				Command:     test.Command("images", "--filter", "label=foo=bar"),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: expect.Contains(data.Labels().Get("builtImageID")),
					}
				},
			},
			{
				Description: "label=foo=bar1",
				Command:     test.Command("images", "--filter", "label=foo=bar1"),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: expect.DoesNotContain(data.Labels().Get("builtImageID")),
					}
				},
			},
			{
				Description: "label=foo=bar label=version=0.1",
				Command:     test.Command("images", "--filter", "label=foo=bar", "--filter", "label=version=0.1"),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: expect.Contains(data.Labels().Get("builtImageID")),
					}
				},
			},
			{
				Description: "label=foo=bar label=version=0.2",
				Command:     test.Command("images", "--filter", "label=foo=bar", "--filter", "label=version=0.2"),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: expect.DoesNotContain(data.Labels().Get("builtImageID")),
					}
				},
			},
			{
				Description: "label=version",
				Command:     test.Command("images", "--filter", "label=version"),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: expect.Contains(data.Labels().Get("builtImageID")),
					}
				},
			},
			{
				Description: "reference=ID*",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("images", "--filter", fmt.Sprintf("reference=%s*", data.Labels().Get("builtImageID")))
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: expect.Contains(data.Labels().Get("builtImageID")),
					}
				},
			},
			{
				Description: "reference=tagged*:*fragment*",
				Command:     test.Command("images", "--filter", "reference=tagged*:*fragment*"),
				Expected: test.Expects(
					0,
					nil,
					expect.Contains("one-", "two-"),
				),
			},
			{
				Description: "before=ID:latest",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("images", "--filter", fmt.Sprintf("before=%s:latest", data.Labels().Get("builtImageID")))
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: expect.All(
							expect.Contains(commonImage.FamiliarName(), commonImage.Tag),
							expect.DoesNotContain(data.Labels().Get("builtImageID")),
						),
					}
				},
			},
			{
				Description: "since=" + commonImage.String(),
				Command:     test.Command("images", "--filter", fmt.Sprintf("since=%s", commonImage.String())),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: expect.All(
							expect.Contains(data.Labels().Get("builtImageID")),
							expect.DoesNotMatch(regexp.MustCompile(commonImage.FamiliarName()+"[\\s]+"+commonImage.Tag)),
						),
					}
				},
			},
			{
				Description: "since=" + commonImage.String() + " " + commonImage.String(),
				Command:     test.Command("images", "--filter", fmt.Sprintf("since=%s", commonImage.String()), commonImage.String()),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: expect.All(
							expect.DoesNotContain(data.Labels().Get("builtImageID")),
							expect.DoesNotMatch(regexp.MustCompile(commonImage.FamiliarName()+"[\\s]+"+commonImage.Tag)),
						),
					}
				},
			},
			{
				Description: "since=non-exists-image",
				Command:     test.Command("images", "--filter", "since=non-exists-image"),
				Expected:    test.Expects(expect.ExitCodeGenericFail, []error{errors.New("no such image: ")}, nil),
			},
			{
				Description: "before=non-exists-image",
				Command:     test.Command("images", "--filter", "before=non-exists-image"),
				Expected:    test.Expects(expect.ExitCodeGenericFail, []error{errors.New("no such image: ")}, nil),
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
			buildCtx := data.Temp().Path()
			data.Temp().Save(dockerfile, "Dockerfile")
			data.Labels().Set("buildCtx", buildCtx)
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("container", "prune", "-f")
			helpers.Anyhow("image", "prune", "--all", "-f")
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("build", data.Labels().Get("buildCtx"))
		},
		Expected: test.Expects(0, nil, nil),
		SubTests: []*test.Case{
			{
				Description: "dangling",
				Command:     test.Command("images", "--filter", "dangling=true"),
				Expected:    test.Expects(0, nil, expect.Contains("<none>")),
			},
			{
				Description: "not dangling",
				Command:     test.Command("images", "--filter", "dangling=false"),
				Expected:    test.Expects(0, nil, expect.DoesNotContain("<none>")),
			},
		},
	}

	testCase.Run(t)
}

func TestImagesKubeWithKubeHideDupe(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Require: require.All(
			nerdtest.OnlyKubernetes,
		),
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("pull", "--quiet", testutil.BusyboxImage)
		},
		SubTests: []*test.Case{
			{
				Description: "The same imageID will not print no-repo:tag in k8s.io with kube-hide-dupe",
				Command:     test.Command("--kube-hide-dupe", "images"),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, t tig.T) {
							var imageID string
							var skipLine int
							lines := strings.Split(strings.TrimSpace(stdout), "\n")
							header := "REPOSITORY\tTAG\tIMAGE ID\tCREATED\tPLATFORM\tSIZE\tBLOB SIZE"
							if nerdtest.IsDocker() {
								header = "REPOSITORY\tTAG\tIMAGE ID\tCREATED\tSIZE"
							}
							tab := tabutil.NewReader(header)
							err := tab.ParseHeader(lines[0])
							assert.NilError(t, err, "ParseHeader should not fail\n")
							found := true
							for i, line := range lines[1:] {
								repo, _ := tab.ReadRow(line, "REPOSITORY")
								tag, _ := tab.ReadRow(line, "TAG")
								if repo+":"+tag == testutil.BusyboxImage {
									skipLine = i
									imageID, _ = tab.ReadRow(line, "IMAGE ID")
									break
								}
							}
							for i, line := range lines[1:] {
								if i == skipLine {
									continue
								}
								id, _ := tab.ReadRow(line, "IMAGE ID")
								if id == imageID {
									found = false
									break
								}
							}
							assert.Assert(t, found, "We should have found the image\n")
						},
					}
				},
			},
			{
				Description: "the same imageId will print no-repo:tag in k8s.io without kube-hide-dupe",
				Command:     test.Command("images"),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: expect.Contains("<none>"),
					}
				},
			},
		},
	}

	testCase.Run(t)
}
