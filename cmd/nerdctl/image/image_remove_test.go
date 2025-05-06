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
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/imgutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestRemove(t *testing.T) {
	testCase := nerdtest.Setup()

	const (
		imgShortIDKey = "imgShortID"
	)

	repoName, _ := imgutil.ParseRepoTag(testutil.CommonImage)
	nginxRepoName, _ := imgutil.ParseRepoTag(testutil.NginxAlpineImage)
	// NOTES:
	// - since all of these are rmi-ing the common image, we need private mode
	testCase.Require = nerdtest.Private

	testCase.SubTests = []*test.Case{
		{
			Description: "Remove image with stopped container - without -f",
			NoParallel:  true,
			Require: require.All(
				require.Not(nerdtest.Docker),
			),
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "--quiet", "--pull", "always", "--name", data.Identifier(), testutil.CommonImage)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: test.Command("rmi", testutil.CommonImage),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New("image is being used")},
					Output: func(stdout string, info string, t *testing.T) {
						helpers.Command("images").Run(&test.Expected{
							Output: expect.Contains(repoName),
						})
					},
				}
			},
		},
		{
			Description: "Remove image with stopped container - with -f",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "--quiet", "--pull", "always", "--name", data.Identifier(), testutil.CommonImage)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: test.Command("rmi", "-f", testutil.CommonImage),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						helpers.Command("images").Run(&test.Expected{
							Output: expect.DoesNotContain(repoName),
						})
					},
				}
			},
		},
		{
			Description: "Remove image with running container - without -f",
			NoParallel:  true,
			Require: require.All(
				require.Not(nerdtest.Docker),
			),
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "--quiet", "--pull", "always", "-d", "--name", data.Identifier(), testutil.CommonImage, "sleep", nerdtest.Infinity)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: test.Command("rmi", testutil.CommonImage),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New("image is being used")},
					Output: func(stdout string, info string, t *testing.T) {
						helpers.Command("images").Run(&test.Expected{
							Output: expect.Contains(repoName),
						})
					},
				}
			},
		},
		{
			Description: "Remove image with running container - with -f",
			NoParallel:  true,
			Require: require.All(
				require.Not(nerdtest.Docker),
			),
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "--quiet", "--pull", "always", "-d", "--name", data.Identifier(), testutil.CommonImage, "sleep", nerdtest.Infinity)

				img := nerdtest.InspectImage(helpers, testutil.CommonImage)
				repoName, _ := imgutil.ParseRepoTag(testutil.CommonImage)
				imgShortID := strings.TrimPrefix(img.RepoDigests[0], repoName+"@sha256:")[0:8]

				data.Labels().Set(imgShortIDKey, imgShortID)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
				helpers.Anyhow("rmi", "-f", data.Labels().Get(imgShortIDKey))
			},
			Command: test.Command("rmi", "-f", testutil.CommonImage),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Errors:   []error{},
					Output: func(stdout string, info string, t *testing.T) {
						helpers.Command("images").Run(&test.Expected{
							Output: expect.Contains("<none>"),
						})
					},
				}
			},
		},
		{
			Description: "Remove image with created container - without -f",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("create", "--quiet", "--pull", "always", "--name", data.Identifier(), testutil.CommonImage, "sleep", nerdtest.Infinity)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: test.Command("rmi", testutil.CommonImage),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New("image is being used")},
					Output: func(stdout string, info string, t *testing.T) {
						helpers.Command("images").Run(&test.Expected{
							Output: expect.Contains(repoName),
						})
					},
				}
			},
		},
		{
			Description: "Remove image with created container - with -f",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("pull", "--quiet", testutil.NginxAlpineImage)
				helpers.Ensure("create", "--quiet", "--pull", "always", "--name", data.Identifier(), testutil.CommonImage, "sleep", nerdtest.Infinity)
				helpers.Ensure("rmi", testutil.NginxAlpineImage)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: test.Command("rmi", "-f", testutil.CommonImage),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						helpers.Command("images").Run(&test.Expected{
							// a created container with removed image doesn't impact other `rmi` command
							Output: expect.DoesNotContain(repoName, nginxRepoName),
						})
					},
				}
			},
		},
		{
			Description: "Remove image with paused container - without -f",
			NoParallel:  true,
			Require: require.All(
				require.Not(nerdtest.Docker),
				nerdtest.CGroup,
			),
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "--quiet", "--pull", "always", "-d", "--name", data.Identifier(), testutil.CommonImage, "sleep", nerdtest.Infinity)
				helpers.Ensure("pause", data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: test.Command("rmi", testutil.CommonImage),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New("image is being used")},
					Output: func(stdout string, info string, t *testing.T) {
						helpers.Command("images").Run(&test.Expected{
							Output: expect.Contains(repoName),
						})
					},
				}
			},
		},
		{
			Description: "Remove image with paused container - with -f",
			NoParallel:  true,
			Require: require.All(
				nerdtest.CGroup,
				require.Not(nerdtest.Docker),
			),
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "--quiet", "--pull", "always", "-d", "--name", data.Identifier(), testutil.CommonImage, "sleep", nerdtest.Infinity)
				helpers.Ensure("pause", data.Identifier())

				img := nerdtest.InspectImage(helpers, testutil.CommonImage)
				repoName, _ := imgutil.ParseRepoTag(testutil.CommonImage)
				imgShortID := strings.TrimPrefix(img.RepoDigests[0], repoName+"@sha256:")[0:8]

				data.Labels().Set(imgShortIDKey, imgShortID)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
				helpers.Anyhow("rmi", "-f", data.Labels().Get(imgShortIDKey))
			},
			Command: test.Command("rmi", "-f", testutil.CommonImage),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Errors:   []error{},
					Output: func(stdout string, info string, t *testing.T) {
						helpers.Command("images").Run(&test.Expected{
							Output: expect.Contains("<none>"),
						})
					},
				}
			},
		},
		{
			Description: "Remove image with killed container - without -f",
			NoParallel:  true,
			Require: require.All(
				require.Not(nerdtest.Docker),
			),
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "--quiet", "--pull", "always", "-d", "--name", data.Identifier(), testutil.CommonImage, "sleep", nerdtest.Infinity)
				helpers.Ensure("kill", data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: test.Command("rmi", testutil.CommonImage),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New("image is being used")},
					Output: func(stdout string, info string, t *testing.T) {
						helpers.Command("images").Run(&test.Expected{
							Output: expect.Contains(repoName),
						})
					},
				}
			},
		},
		{
			Description: "Remove image with killed container - with -f",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "--quiet", "--pull", "always", "-d", "--name", data.Identifier(), testutil.CommonImage, "sleep", nerdtest.Infinity)
				helpers.Ensure("kill", data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: test.Command("rmi", "-f", testutil.CommonImage),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						helpers.Command("images").Run(&test.Expected{
							Output: expect.DoesNotContain(repoName),
						})
					},
				}
			},
		},
	}

	testCase.Run(t)
}

// TestIssue3016 tests https://github.com/containerd/nerdctl/issues/3016
func TestIssue3016(t *testing.T) {
	testCase := nerdtest.Setup()

	const (
		tagIDKey = "tagID"
	)

	testCase.SubTests = []*test.Case{
		{
			Description: "Issue #3016 - Tags created using the short digest ids of container images cannot be deleted using the nerdctl rmi command.",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("pull", "--quiet", testutil.CommonImage)
				helpers.Ensure("pull", "--quiet", testutil.NginxAlpineImage)

				img := nerdtest.InspectImage(helpers, testutil.NginxAlpineImage)
				repoName, _ := imgutil.ParseRepoTag(testutil.NginxAlpineImage)
				tagID := strings.TrimPrefix(img.RepoDigests[0], repoName+"@sha256:")[0:8]

				helpers.Ensure("tag", testutil.CommonImage, tagID)

				data.Labels().Set(tagIDKey, tagID)
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("rmi", data.Labels().Get(tagIDKey))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Errors:   []error{},
					Output: func(stdout string, info string, t *testing.T) {
						helpers.Command("images", data.Labels().Get(tagIDKey)).Run(&test.Expected{
							ExitCode: 0,
							Output: func(stdout string, info string, t *testing.T) {
								assert.Equal(t, len(strings.Split(stdout, "\n")), 2)
							},
						})
					},
				}
			},
		},
	}

	testCase.Run(t)
}

func TestRemoveKubeWithKubeHideDupe(t *testing.T) {
	var numTags, numNoTags int
	testCase := nerdtest.Setup()
	testCase.NoParallel = true
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("--kube-hide-dupe", "rmi", "-f", testutil.BusyboxImage)
	}
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		numTags = len(strings.Split(strings.TrimSpace(helpers.Capture("--kube-hide-dupe", "images")), "\n"))
		numNoTags = len(strings.Split(strings.TrimSpace(helpers.Capture("images")), "\n"))
	}
	testCase.Require = require.All(
		nerdtest.OnlyKubernetes,
	)
	testCase.SubTests = []*test.Case{
		{
			Description: "After removing the tag without kube-hide-dupe, repodigest is shown as <none>",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("pull", "--quiet", testutil.BusyboxImage)
			},
			Command: test.Command("rmi", "-f", testutil.BusyboxImage),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Errors:   []error{},
					Output: func(stdout string, info string, t *testing.T) {
						helpers.Command("--kube-hide-dupe", "images").Run(&test.Expected{
							Output: func(stdout string, info string, t *testing.T) {
								lines := strings.Split(strings.TrimSpace(stdout), "\n")
								assert.Assert(t, len(lines) == numTags+1, info)
							},
						})
						helpers.Command("images").Run(&test.Expected{
							Output: func(stdout string, info string, t *testing.T) {
								lines := strings.Split(strings.TrimSpace(stdout), "\n")
								assert.Assert(t, len(lines) == numNoTags+1, info)
							},
						})
					},
				}
			},
		},
		{
			Description: "If there are other tags, the Repodigest will not be deleted",
			NoParallel:  true,
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("--kube-hide-dupe", "rmi", data.Identifier())
			},
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("pull", "--quiet", testutil.BusyboxImage)
				helpers.Ensure("tag", testutil.BusyboxImage, data.Identifier())
			},
			Command: test.Command("--kube-hide-dupe", "rmi", testutil.BusyboxImage),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Errors:   []error{},
					Output: func(stdout string, info string, t *testing.T) {
						helpers.Command("--kube-hide-dupe", "images").Run(&test.Expected{
							Output: func(stdout string, info string, t *testing.T) {
								lines := strings.Split(strings.TrimSpace(stdout), "\n")
								assert.Assert(t, len(lines) == numTags+1, info)
							},
						})
						helpers.Command("images").Run(&test.Expected{
							Output: func(stdout string, info string, t *testing.T) {
								lines := strings.Split(strings.TrimSpace(stdout), "\n")
								assert.Assert(t, len(lines) == numNoTags+2, info)
							},
						})
					},
				}
			},
		},
		{
			Description: "After deleting all repo:tag entries, all repodigests will be cleaned up",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("pull", "--quiet", testutil.BusyboxImage)
				helpers.Ensure("tag", testutil.BusyboxImage, data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				helpers.Ensure("--kube-hide-dupe", "rmi", "-f", testutil.BusyboxImage)
				return helpers.Command("--kube-hide-dupe", "rmi", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						helpers.Command("--kube-hide-dupe", "images").Run(&test.Expected{
							Output: func(stdout string, info string, t *testing.T) {
								lines := strings.Split(strings.TrimSpace(stdout), "\n")
								assert.Assert(t, len(lines) == numTags, info)
							},
						})
						helpers.Command("images").Run(&test.Expected{
							Output: func(stdout string, info string, t *testing.T) {
								lines := strings.Split(strings.TrimSpace(stdout), "\n")
								assert.Assert(t, len(lines) == numNoTags, info)
							},
						})
					},
				}
			},
		},
		{
			Description: "Test multiple IDs found with provided prefix and force with shortID",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("pull", "--quiet", testutil.BusyboxImage)
				helpers.Ensure("tag", testutil.BusyboxImage, data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("--kube-hide-dupe", "images", testutil.BusyboxImage, "-q")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						helpers.Command("--kube-hide-dupe", "rmi", stdout[0:12]).Run(&test.Expected{
							ExitCode: 1,
							Errors:   []error{errors.New("multiple IDs found with provided prefix: ")},
						})
						helpers.Command("--kube-hide-dupe", "rmi", "--force", stdout[0:12]).Run(&test.Expected{
							ExitCode: 0,
						})
						helpers.Command("images").Run(&test.Expected{
							Output: func(stdout string, info string, t *testing.T) {
								lines := strings.Split(strings.TrimSpace(stdout), "\n")
								assert.Assert(t, len(lines) == numNoTags, info)
							},
						})
					},
				}
			},
		},
		{
			Description: "Test remove image with digestID",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("pull", "--quiet", testutil.BusyboxImage)
				helpers.Ensure("tag", testutil.BusyboxImage, data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("--kube-hide-dupe", "images", testutil.BusyboxImage, "-q", "--no-trunc")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						imgID := strings.Split(stdout, "\n")
						helpers.Command("--kube-hide-dupe", "rmi", imgID[0]).Run(&test.Expected{
							ExitCode: 1,
							Errors:   []error{errors.New("multiple IDs found with provided prefix: ")},
						})
						helpers.Command("--kube-hide-dupe", "rmi", "--force", imgID[0]).Run(&test.Expected{
							ExitCode: 0,
						})
						helpers.Command("images").Run(&test.Expected{
							Output: func(stdout string, info string, t *testing.T) {
								lines := strings.Split(strings.TrimSpace(stdout), "\n")
								assert.Assert(t, len(lines) == numNoTags, info)
							},
						})
					},
				}
			},
		},
	}
	testCase.Run(t)
}
