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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	testhelpers "github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestSaveContent(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		// FIXME: move to busybox for windows?
		Require: require.Not(require.Windows),
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("pull", "--quiet", testutil.CommonImage)
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("save", "-o", filepath.Join(data.Temp().Path(), "out.tar"), testutil.CommonImage)
		},
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			return &test.Expected{
				Output: func(stdout string, info string, t *testing.T) {
					rootfsPath := filepath.Join(data.Temp().Path(), "rootfs")
					err := testhelpers.ExtractDockerArchive(filepath.Join(data.Temp().Path(), "out.tar"), rootfsPath)
					assert.NilError(t, err)
					etcOSReleasePath := filepath.Join(rootfsPath, "/etc/os-release")
					etcOSReleaseBytes, err := os.ReadFile(etcOSReleasePath)
					assert.NilError(t, err)
					etcOSRelease := string(etcOSReleaseBytes)
					assert.Assert(t, strings.Contains(etcOSRelease, "Alpine"))
				},
			}
		},
	}

	testCase.Run(t)
}

func TestSave(t *testing.T) {
	testCase := nerdtest.Setup()

	// This test relies on the fact that we can remove the common image, which definitely conflicts with others,
	// hence the private mode.
	// Further note though, that this will hide the fact this the save command could fail if some layers are missing.
	// See https://github.com/containerd/nerdctl/issues/3425 and others for details.
	testCase.Require = nerdtest.Private

	if runtime.GOOS == "windows" {
		testCase.Require = nerdtest.IsFlaky("https://github.com/containerd/nerdctl/issues/3524")
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "Single image, by id",
			NoParallel:  true,
			Cleanup: func(data test.Data, helpers test.Helpers) {
				if data.Labels().Get("id") != "" {
					helpers.Anyhow("rmi", "-f", data.Labels().Get("id"))
				}
			},
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("pull", "--quiet", testutil.CommonImage)
				img := nerdtest.InspectImage(helpers, testutil.CommonImage)
				var id string
				// Docker and Nerdctl do not agree on what is the definition of an image ID
				if nerdtest.IsDocker() {
					id = img.ID
				} else {
					id = strings.Split(img.RepoDigests[0], ":")[1]
				}
				tarPath := filepath.Join(data.Temp().Path(), "out.tar")
				helpers.Ensure("save", "-o", tarPath, id)
				helpers.Ensure("rmi", "-f", testutil.CommonImage)
				helpers.Ensure("load", "-i", tarPath)
				data.Labels().Set("id", id)
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", data.Labels().Get("id"), "sh", "-euxc", "echo foo")
			},
			Expected: test.Expects(0, nil, expect.Equals("foo\n")),
		},
		{
			Description: "Image with different names, by id",
			NoParallel:  true,
			Cleanup: func(data test.Data, helpers test.Helpers) {
				if data.Labels().Get("id") != "" {
					helpers.Anyhow("rmi", "-f", data.Labels().Get("id"))
				}
			},
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("pull", "--quiet", testutil.CommonImage)
				img := nerdtest.InspectImage(helpers, testutil.CommonImage)
				var id string
				if nerdtest.IsDocker() {
					id = img.ID
				} else {
					id = strings.Split(img.RepoDigests[0], ":")[1]
				}
				helpers.Ensure("tag", testutil.CommonImage, data.Identifier())
				tarPath := filepath.Join(data.Temp().Path(), "out.tar")
				helpers.Ensure("save", "-o", tarPath, id)
				helpers.Ensure("rmi", "-f", testutil.CommonImage)
				helpers.Ensure("load", "-i", tarPath)
				data.Labels().Set("id", id)
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", data.Labels().Get("id"), "sh", "-euxc", "echo foo")
			},
			Expected: test.Expects(0, nil, expect.Equals("foo\n")),
		},
	}

	testCase.Run(t)
}

// TestSaveMultipleImagesWithSameIDAndLoad tests https://github.com/containerd/nerdctl/issues/3806
func TestSaveMultipleImagesWithSameIDAndLoad(t *testing.T) {
	testCase := nerdtest.Setup()

	// This test relies on the fact that we can remove the common image, which definitely conflicts with others,
	// hence the private mode.
	// Further note though, that this will hide the fact this the save command could fail if some layers are missing.
	// See https://github.com/containerd/nerdctl/issues/3425 and others for details.
	testCase.Require = nerdtest.Private

	if runtime.GOOS == "windows" {
		testCase.Require = nerdtest.IsFlaky("https://github.com/containerd/nerdctl/issues/3524")
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "Issue #3568 - Save multiple container images with the same image ID but different image names",
			NoParallel:  true,
			Cleanup: func(data test.Data, helpers test.Helpers) {
				if data.Labels().Get("id") != "" {
					helpers.Anyhow("rmi", "-f", data.Labels().Get("id"))
				}
			},
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("pull", "--quiet", testutil.CommonImage)
				img := nerdtest.InspectImage(helpers, testutil.CommonImage)
				var id string
				if nerdtest.IsDocker() {
					id = img.ID
				} else {
					id = strings.Split(img.RepoDigests[0], ":")[1]
				}
				helpers.Ensure("tag", testutil.CommonImage, data.Identifier())
				tarPath := filepath.Join(data.Temp().Path(), "out.tar")
				helpers.Ensure("save", "-o", tarPath, testutil.CommonImage, data.Identifier())
				helpers.Ensure("rmi", "-f", id)
				helpers.Ensure("load", "-i", tarPath)
				data.Labels().Set("id", id)
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("images", "--no-trunc")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Errors:   []error{},
					Output: func(stdout string, info string, t *testing.T) {
						assert.Equal(t, strings.Count(stdout, data.Labels().Get("id")), 2)
					},
				}
			},
		},
	}

	testCase.Run(t)
}
