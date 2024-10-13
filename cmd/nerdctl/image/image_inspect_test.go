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
	"encoding/json"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestImageInspectSimpleCases(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Description: "TestImageInspect",
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("pull", testutil.CommonImage)
		},
		SubTests: []*test.Case{
			{
				Description: "Contains some stuff",
				Command:     test.Command("image", "inspect", testutil.CommonImage),
				Expected: test.Expects(0, nil, func(stdout string, info string, t *testing.T) {
					var dc []dockercompat.Image
					err := json.Unmarshal([]byte(stdout), &dc)
					assert.NilError(t, err, "Unable to unmarshal output\n"+info)
					assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results\n"+info)
					assert.Assert(t, len(dc[0].RootFS.Layers) > 0, info)
					assert.Assert(t, dc[0].Architecture != "", info)
					assert.Assert(t, dc[0].Size > 0, info)
				}),
			},
			{
				Description: "RawFormat support (.Id)",
				Command:     test.Command("image", "inspect", testutil.CommonImage, "--format", "{{.Id}}"),
				Expected:    test.Expects(0, nil, nil),
			},
			{
				Description: "typedFormat support (.ID)",
				Command:     test.Command("image", "inspect", testutil.CommonImage, "--format", "{{.ID}}"),
				Expected:    test.Expects(0, nil, nil),
			},
		},
	}

	testCase.Run(t)
}

func TestImageInspectDifferentValidReferencesForTheSameImage(t *testing.T) {
	nerdtest.Setup()

	tags := []string{
		"",
		":latest",
	}
	names := []string{
		"busybox",
		"docker.io/library/busybox",
		"registry-1.docker.io/library/busybox",
	}

	testCase := &test.Case{
		Require: test.Require(
			test.Not(nerdtest.Docker),
			test.Not(test.Windows),
			// We need a clean slate
			nerdtest.Private,
		),
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("pull", "alpine")
			helpers.Ensure("pull", "busybox")
			helpers.Ensure("pull", "registry-1.docker.io/library/busybox")
		},
		SubTests: []*test.Case{
			{
				Description: "name and tags +/- sha combinations",
				Command:     test.Command("image", "inspect", "busybox"),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							var dc []dockercompat.Image
							err := json.Unmarshal([]byte(stdout), &dc)
							assert.NilError(t, err, "Unable to unmarshal output\n"+info)
							assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results\n"+info)
							reference := dc[0].ID
							sha := strings.TrimPrefix(dc[0].RepoDigests[0], "busybox@sha256:")

							for _, name := range names {
								for _, tag := range tags {
									it := nerdtest.InspectImage(helpers, name+tag)
									assert.Equal(t, it.ID, reference)
									it = nerdtest.InspectImage(helpers, name+tag+"@sha256:"+sha)
									assert.Equal(t, it.ID, reference)
								}
							}
						},
					}
				},
			},
			{
				Description: "by digest, short or long, with or without prefix",
				Command:     test.Command("image", "inspect", "busybox"),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							var dc []dockercompat.Image
							err := json.Unmarshal([]byte(stdout), &dc)
							assert.NilError(t, err, "Unable to unmarshal output\n"+info)
							assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results\n"+info)
							reference := dc[0].ID
							sha := strings.TrimPrefix(dc[0].RepoDigests[0], "busybox@sha256:")

							for _, id := range []string{"sha256:" + sha, sha, sha[0:8], "sha256:" + sha[0:8]} {
								it := nerdtest.InspectImage(helpers, id)
								assert.Equal(t, it.ID, reference)
							}

							// Now, tag alpine with a short id
							// Build reference values for comparison
							alpine := nerdtest.InspectImage(helpers, "alpine")

							// Demonstrate image name precedence over digest lookup
							// Using the shortened sha should no longer get busybox, but rather the newly tagged Alpine
							// FIXME: this is triggering https://github.com/containerd/nerdctl/issues/3016
							// We cannot get rid of that image now, which does break local testing
							helpers.Ensure("tag", "alpine", sha[0:8])
							it := nerdtest.InspectImage(helpers, sha[0:8])
							assert.Equal(t, it.ID, alpine.ID)
						},
					}
				},
			},
			{
				Description: "prove that wrong references with correct digest do not get resolved",
				Command:     test.Command("image", "inspect", "busybox"),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							var dc []dockercompat.Image
							err := json.Unmarshal([]byte(stdout), &dc)
							assert.NilError(t, err, "Unable to unmarshal output\n"+info)
							assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results\n"+info)
							sha := strings.TrimPrefix(dc[0].RepoDigests[0], "busybox@sha256:")

							for _, id := range []string{"doesnotexist", "doesnotexist:either", "busybox:bogustag"} {
								cmd := helpers.Command("image", "inspect", id+"@sha256:"+sha)
								cmd.Run(&test.Expected{
									Output: test.Equals(""),
								})
							}
						},
					}
				},
			},
			{
				Description: "prove that invalid reference return no result without crashing",
				Command:     test.Command("image", "inspect", "busybox"),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							var dc []dockercompat.Image
							err := json.Unmarshal([]byte(stdout), &dc)
							assert.NilError(t, err, "Unable to unmarshal output\n"+info)
							assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results\n"+info)

							for _, id := range []string{"∞∞∞∞∞∞∞∞∞∞", "busybox:∞∞∞∞∞∞∞∞∞∞"} {
								cmd := helpers.Command("image", "inspect", id)
								cmd.Run(&test.Expected{
									Output: test.Equals(""),
								})
							}
						},
					}
				},
			},
			{
				Description: "retrieving multiple entries at once",
				Command:     test.Command("image", "inspect", "busybox", "busybox"),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							var dc []dockercompat.Image
							err := json.Unmarshal([]byte(stdout), &dc)
							assert.NilError(t, err, "Unable to unmarshal output\n"+info)
							assert.Equal(t, 2, len(dc), "Unexpectedly did not get 2 results\n"+info)
							reference := nerdtest.InspectImage(helpers, "busybox")
							assert.Equal(t, dc[0].ID, reference.ID)
							assert.Equal(t, dc[1].ID, reference.ID)
						},
					}
				},
			},
		},
	}

	testCase.Run(t)
}
