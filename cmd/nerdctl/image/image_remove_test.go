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

	"github.com/containerd/nerdctl/v2/pkg/imgutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestRemove(t *testing.T) {
	testCase := nerdtest.Setup()

	repoName, _ := imgutil.ParseRepoTag(testutil.CommonImage)
	nginxRepoName, _ := imgutil.ParseRepoTag(testutil.NginxAlpineImage)
	// NOTES:
	// - since all of these are rmi-ing the common image, we need private mode
	testCase.Require = nerdtest.Private

	testCase.SubTests = []*test.Case{
		{
			Description: "Remove image with stopped container - without -f",
			NoParallel:  true,
			Require: test.Require(
				test.Not(test.Windows),
				test.Not(nerdtest.Docker),
			),
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "--pull", "always", "--name", data.Identifier(), testutil.CommonImage)
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
							Output: test.Contains(repoName),
						})
					},
				}
			},
		},
		{
			Description: "Remove image with stopped container - with -f",
			NoParallel:  true,
			Require:     test.Not(test.Windows),
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "--pull", "always", "--name", data.Identifier(), testutil.CommonImage)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: test.Command("rmi", "-f", testutil.CommonImage),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						helpers.Command("images").Run(&test.Expected{
							Output: test.DoesNotContain(repoName),
						})
					},
				}
			},
		},
		{
			Description: "Remove image with running container - without -f",
			NoParallel:  true,
			Require: test.Require(
				test.Not(test.Windows),
				test.Not(nerdtest.Docker),
			),
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "--pull", "always", "-d", "--name", data.Identifier(), testutil.CommonImage, "sleep", "infinity")
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
							Output: test.Contains(repoName),
						})
					},
				}
			},
		},
		{
			Description: "Remove image with running container - with -f",
			NoParallel:  true,
			Require: test.Require(
				test.Not(test.Windows),
			),
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "--pull", "always", "-d", "--name", data.Identifier(), testutil.CommonImage, "sleep", "infinity")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: test.Command("rmi", "-f", testutil.CommonImage),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						helpers.Command("images").Run(&test.Expected{
							Output: test.DoesNotContain(repoName),
						})
					},
				}
			},
		},
		{
			Description: "Remove image with created container - without -f",
			NoParallel:  true,
			Require:     test.Not(test.Windows),
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("create", "--pull", "always", "--name", data.Identifier(), testutil.CommonImage, "sleep", "infinity")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: test.Command("rmi", "-f", testutil.CommonImage),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						helpers.Command("images").Run(&test.Expected{
							Output: test.DoesNotContain(repoName),
						})
					},
				}
			},
		},
		{
			Description: "Remove image with created container - with -f",
			NoParallel:  true,
			Require:     test.Not(test.Windows),
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("pull", "--quiet", testutil.NginxAlpineImage)
				helpers.Ensure("create", "--pull", "always", "--name", data.Identifier(), testutil.CommonImage, "sleep", "infinity")
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
							Output: test.All(
								test.DoesNotContain(repoName),
								// a created container with removed image doesn't impact other `rmi` command
								test.DoesNotContain(nginxRepoName),
							),
						})
					},
				}
			},
		},
		{
			Description: "Remove image with paused container - without -f",
			NoParallel:  true,
			Require: test.Require(
				test.Not(test.Windows),
				test.Not(nerdtest.Docker),
				nerdtest.CGroup,
			),
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "--pull", "always", "-d", "--name", data.Identifier(), testutil.CommonImage, "sleep", "infinity")
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
							Output: test.Contains(repoName),
						})
					},
				}
			},
		},
		{
			Description: "Remove image with paused container - with -f",
			NoParallel:  true,
			Require: test.Require(
				test.Not(test.Windows),
				nerdtest.CGroup,
			),
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "--pull", "always", "-d", "--name", data.Identifier(), testutil.CommonImage, "sleep", "infinity")
				helpers.Ensure("pause", data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: test.Command("rmi", "-f", testutil.CommonImage),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						helpers.Command("images").Run(&test.Expected{
							Output: test.DoesNotContain(repoName),
						})
					},
				}
			},
		},
		{
			Description: "Remove image with killed container - without -f",
			NoParallel:  true,
			Require: test.Require(
				test.Not(test.Windows),
				test.Not(nerdtest.Docker),
			),
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "--pull", "always", "-d", "--name", data.Identifier(), testutil.CommonImage, "sleep", "infinity")
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
							Output: test.Contains(repoName),
						})
					},
				}
			},
		},
		{
			Description: "Remove image with killed container - with -f",
			NoParallel:  true,
			Require:     test.Not(test.Windows),
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "--pull", "always", "-d", "--name", data.Identifier(), testutil.CommonImage, "sleep", "infinity")
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
							Output: test.DoesNotContain(repoName),
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
				helpers.Ensure("pull", testutil.CommonImage)
				helpers.Ensure("pull", testutil.NginxAlpineImage)

				img := nerdtest.InspectImage(helpers, testutil.NginxAlpineImage)
				repoName, _ := imgutil.ParseRepoTag(testutil.NginxAlpineImage)
				tagID := strings.TrimPrefix(img.RepoDigests[0], repoName+"@sha256:")[0:8]

				helpers.Ensure("tag", testutil.CommonImage, tagID)

				data.Set(tagIDKey, tagID)
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("rmi", data.Get(tagIDKey))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Errors: []error{},
					Output: func(stdout string, info string, t *testing.T) {
						helpers.Command("images", data.Get(tagIDKey)).Run(&test.Expected{
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
