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
	"testing"

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
			Command: test.RunCommand("rmi", testutil.CommonImage),
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
			Command: test.RunCommand("rmi", "-f", testutil.CommonImage),
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
			Command: test.RunCommand("rmi", testutil.CommonImage),
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
			// FIXME: nerdctl is broken
			// https://github.com/containerd/nerdctl/issues/3454
			// If an image is associated with a running/paused containers, `docker rmi -f imageName`
			// untags `imageName` (left a `<none>` image) without deletion; `docker rmi -rf imageID` fails.
			// In both cases, `nerdctl rmi -f` will fail.
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
			Command: test.RunCommand("rmi", "-f", testutil.CommonImage),
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
			Description: "Remove image with created container - without -f",
			NoParallel:  true,
			Require:     test.Not(test.Windows),
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("create", "--pull", "always", "--name", data.Identifier(), testutil.CommonImage, "sleep", "infinity")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: test.RunCommand("rmi", "-f", testutil.CommonImage),
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
				helpers.Ensure("pull", testutil.NginxAlpineImage)
				helpers.Ensure("create", "--pull", "always", "--name", data.Identifier(), testutil.CommonImage, "sleep", "infinity")
				helpers.Ensure("rmi", testutil.NginxAlpineImage)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: test.RunCommand("rmi", "-f", testutil.CommonImage),
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
			Command: test.RunCommand("rmi", testutil.CommonImage),
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
				// FIXME: nerdctl is broken
				// https://github.com/containerd/nerdctl/issues/3454
				// If an image is associated with a running/paused containers, `docker rmi -f imageName`
				// untags `imageName` (left a `<none>` image) without deletion; `docker rmi -rf imageID` fails.
				// In both cases, `nerdctl rmi -f` will fail.
				test.Not(nerdtest.Docker),
			),
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "--pull", "always", "-d", "--name", data.Identifier(), testutil.CommonImage, "sleep", "infinity")
				helpers.Ensure("pause", data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: test.RunCommand("rmi", "-f", testutil.CommonImage),
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
			Command: test.RunCommand("rmi", testutil.CommonImage),
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
			Command: test.RunCommand("rmi", "-f", testutil.CommonImage),
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
