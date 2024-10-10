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
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestRemoveImage(t *testing.T) {
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)
	base.Cmd("image", "prune", "--force", "--all").AssertOK()

	// ignore error
	base.Cmd("rmi", "-f", tID).AssertOK()

	base.Cmd("run", "--name", tID, testutil.CommonImage).AssertOK()
	defer base.Cmd("rm", "-f", tID).AssertOK()

	base.Cmd("rmi", testutil.CommonImage).AssertFail()
	defer base.Cmd("rmi", "-f", testutil.CommonImage).Run()
	base.Cmd("rmi", "-f", testutil.CommonImage).AssertOK()

	base.Cmd("images").AssertOutNotContains(testutil.ImageRepo(testutil.CommonImage))
}

func TestRemoveRunningImage(t *testing.T) {
	// If an image is associated with a running/paused containers, `docker rmi -f imageName`
	// untags `imageName` (left a `<none>` image) without deletion; `docker rmi -rf imageID` fails.
	// In both cases, `nerdctl rmi -f` will fail.
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)

	base.Cmd("run", "--name", tID, "-d", testutil.CommonImage, "sleep", "infinity").AssertOK()
	defer base.Cmd("rm", "-f", tID).AssertOK()

	base.Cmd("rmi", testutil.CommonImage).AssertFail()
	base.Cmd("rmi", "-f", testutil.CommonImage).AssertFail()
	base.Cmd("images").AssertOutContains(testutil.ImageRepo(testutil.CommonImage))

	base.Cmd("kill", tID).AssertOK()
	base.Cmd("rmi", testutil.CommonImage).AssertFail()
	base.Cmd("rmi", "-f", testutil.CommonImage).AssertOK()
	base.Cmd("images").AssertOutNotContains(testutil.ImageRepo(testutil.CommonImage))
}

func TestRemovePausedImage(t *testing.T) {
	// If an image is associated with a running/paused containers, `docker rmi -f imageName`
	// untags `imageName` (left a `<none>` image) without deletion; `docker rmi -rf imageID` fails.
	// In both cases, `nerdctl rmi -f` will fail.
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	switch base.Info().CgroupDriver {
	case "none", "":
		t.Skip("requires cgroup (for pausing)")
	}
	tID := testutil.Identifier(t)

	base.Cmd("run", "--name", tID, "-d", testutil.CommonImage, "sleep", "infinity").AssertOK()
	base.Cmd("pause", tID).AssertOK()
	defer base.Cmd("rm", "-f", tID).AssertOK()

	base.Cmd("rmi", testutil.CommonImage).AssertFail()
	base.Cmd("rmi", "-f", testutil.CommonImage).AssertFail()
	base.Cmd("images").AssertOutContains(testutil.ImageRepo(testutil.CommonImage))

	base.Cmd("kill", tID).AssertOK()
	base.Cmd("rmi", testutil.CommonImage).AssertFail()
	base.Cmd("rmi", "-f", testutil.CommonImage).AssertOK()
	base.Cmd("images").AssertOutNotContains(testutil.ImageRepo(testutil.CommonImage))
}

func TestRemoveImageWithCreatedContainer(t *testing.T) {
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)

	base.Cmd("pull", testutil.AlpineImage).AssertOK()
	base.Cmd("pull", testutil.NginxAlpineImage).AssertOK()

	base.Cmd("create", "--name", tID, testutil.AlpineImage, "sleep", "infinity").AssertOK()
	defer base.Cmd("rm", "-f", tID).AssertOK()

	base.Cmd("rmi", testutil.AlpineImage).AssertFail()
	base.Cmd("rmi", "-f", testutil.AlpineImage).AssertOK()
	base.Cmd("images").AssertOutNotContains(testutil.ImageRepo(testutil.AlpineImage))

	// a created container with removed image doesn't impact other `rmi` command
	base.Cmd("rmi", "-f", testutil.NginxAlpineImage).AssertOK()
	base.Cmd("images").AssertOutNotContains(testutil.ImageRepo(testutil.NginxAlpineImage))
}

// TestIssue3016 tests https://github.com/containerd/nerdctl/issues/3016
func TestIssue3016(t *testing.T) {
	nerdtest.Setup()

	const (
		alpineImageName  = "alpine"
		busyboxImageName = "busybox"
		tagIDKey         = "tagID"
	)

	testCase := &test.Group{
		{
			Description: "Issue #3016 - Tags created using the short digest ids of container images cannot be deleted using the nerdctl rmi command.",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("pull", alpineImageName)
				helpers.Ensure("pull", busyboxImageName)

				img := nerdtest.InspectImage(helpers, busyboxImageName)
				tagID := strings.TrimPrefix(img.RepoDigests[0], "busybox@sha256:")[0:8]
				assert.Equal(t, len(tagID), 8)

				helpers.Ensure("tag", alpineImageName, tagID)

				data.Set(tagIDKey, tagID)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rmi", alpineImageName)
				helpers.Anyhow("rmi", busyboxImageName)
			},
			Command: func(data test.Data, helpers test.Helpers) test.Command {
				return helpers.Command("rmi", data.Get(tagIDKey))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Errors:   []error{},
					Output: func(stdout string, info string, t *testing.T) {
						helpers.Command("images", data.Get(tagIDKey)).Run(&test.Expected{
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
