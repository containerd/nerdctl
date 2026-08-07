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
	"slices"
	"strings"
	"testing"

	"github.com/docker/go-units"
	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"
	"github.com/containerd/platforms"

	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
	"github.com/containerd/nerdctl/v2/pkg/tabutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

// treeImageKey is the testutil registry key of testutil.CommonImage. Its entry declares the
// platforms of the image and the expected content size of each of them.
const treeImageKey = "alpine"

// treeChildFields splits a per-platform row of `image ls --tree` into its cells, returning nil for
// any other line. The rows are split on whitespace rather than read with tabutil, because tabutil
// indexes the columns by byte offset while the branch glyphs are multi-byte: the tabwriter aligns
// them by rune, so the byte offsets of a child row no longer match the header's.
func treeChildFields(line string) []string {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "├─") && !strings.HasPrefix(trimmed, "└─") {
		return nil
	}
	// ["├─", "linux/amd64", "<id>", "<disk usage>", "<content size>", optional "U"]
	return strings.Fields(trimmed)
}

// normalizeTreePlatform renders a platform the way the testutil registry keys it, so that a row can
// be looked up whatever form the tested binary printed it in (docker keeps the "v8" variant of
// linux/arm64, nerdctl normalizes it away).
func normalizeTreePlatform(platform string) string {
	parsed, err := platforms.Parse(platform)
	if err != nil {
		return platform
	}
	return platforms.Format(platforms.Normalize(parsed))
}

func TestImagesTree(t *testing.T) {
	nerdtest.Setup()

	commonImage, _ := referenceutil.Parse(testutil.CommonImage)
	imageRef := commonImage.FamiliarName() + ":" + commonImage.Tag
	hostPlatform := platforms.Format(platforms.Normalize(platforms.DefaultSpec()))
	treeHeader := "IMAGE\tID\tDISK USAGE\tCONTENT SIZE\tEXTRA"

	testCase := &test.Case{
		Setup: func(data test.Data, helpers test.Helpers) {
			if nerdtest.IsDocker() {
				// `docker pull` has no --all-platforms, so only the host platform is available there.
				helpers.Ensure("pull", "--quiet", commonImage.String())
				return
			}
			helpers.Ensure("pull", "--quiet", "--all-platforms", commonImage.String())
		},
		SubTests: []*test.Case{
			{
				Description: "a row per platform, with the sizes of the content store",
				Command:     test.Command("images", "--tree", commonImage.String()),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, t tig.T) {
							lines := strings.Split(strings.TrimSpace(stdout), "\n")
							assert.Assert(t, len(lines) >= 3,
								"expected a header, an image row and at least one platform row\n")

							tab := tabutil.NewReader(treeHeader)
							err := tab.ParseHeader(lines[0])
							assert.NilError(t, err, "ParseHeader should not fail\n")

							known := testutil.GetTestImagePlatforms(treeImageKey)
							foundImage := false
							var seen, notPulled []string
							for _, line := range lines[1:] {
								fields := treeChildFields(line)
								if fields == nil {
									if image, _ := tab.ReadRow(line, "IMAGE"); image == imageRef {
										foundImage = true
									}
									continue
								}
								assert.Assert(t, len(fields) >= 5,
									"a platform row should have all its columns, got %q\n", line)

								platform := normalizeTreePlatform(fields[1])
								assert.Assert(t, slices.Contains(known, platform),
									"unexpected platform %q, the testutil registry knows %v\n", platform, known)
								seen = append(seen, platform)

								assert.Equal(t, len(fields[2]), 12,
									"a platform row should carry a truncated ID\n")

								diskUsage, err := units.FromHumanSize(fields[3])
								assert.NilError(t, err, "DISK USAGE of %s is %q\n", platform, fields[3])
								contentSize, err := units.FromHumanSize(fields[4])
								assert.NilError(t, err, "CONTENT SIZE of %s is %q\n", platform, fields[4])

								// DISK USAGE adds the unpacked snapshots on top of the content. Which
								// platforms are unpacked depends on what the rest of the suite did with
								// the shared image store (TestMultiPlatformRun runs this very image on
								// several platforms), so only the invariant can be asserted.
								assert.Assert(t, diskUsage >= contentSize,
									"DISK USAGE (%d) should cover CONTENT SIZE (%d) of %s\n",
									diskUsage, contentSize, platform)

								if contentSize == 0 {
									// The index lists every platform of the image, including the ones
									// that were never pulled: those have no content to size.
									notPulled = append(notPulled, platform)
									continue
								}
								// CONTENT SIZE is the size of the blobs, which is fixed for a given
								// image, so it can be checked exactly.
								assert.Equal(t, fields[4], units.HumanSizeWithPrecision(
									float64(testutil.GetTestImageContentSize(treeImageKey, platform)), 3),
									"CONTENT SIZE of %s\n", platform)
							}

							assert.Assert(t, foundImage, "we should have found the image row\n")

							// Every platform the index declares is listed, whether it was pulled or not.
							slices.Sort(seen)
							assert.DeepEqual(t, seen, known)

							if nerdtest.IsDocker() {
								// `docker pull` could only fetch the host platform, see Setup.
								assert.Assert(t, !slices.Contains(notPulled, hostPlatform),
									"the host platform should have been pulled, %v were not\n", notPulled)
								return
							}
							assert.Assert(t, len(notPulled) == 0,
								"--all-platforms should have pulled every platform, but %v have no content\n",
								notPulled)
						},
					}
				},
			},
			{
				Description: "flags the platform a container runs",
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("create", "--name", data.Identifier(),
						commonImage.String(), "sleep", nerdtest.Infinity)
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rm", "-f", data.Identifier())
				},
				Command: test.Command("images", "--tree", commonImage.String()),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, t tig.T) {
							lines := strings.Split(strings.TrimSpace(stdout), "\n")
							tab := tabutil.NewReader(treeHeader)
							err := tab.ParseHeader(lines[0])
							assert.NilError(t, err, "ParseHeader should not fail\n")

							imageInUse := false
							var platformsInUse []string
							for _, line := range lines[1:] {
								fields := treeChildFields(line)
								if fields == nil {
									if image, _ := tab.ReadRow(line, "IMAGE"); image == imageRef {
										extra, _ := tab.ReadRow(line, "EXTRA")
										imageInUse = extra == "U"
									}
									continue
								}
								if len(fields) >= 6 && fields[5] == "U" {
									platformsInUse = append(platformsInUse, normalizeTreePlatform(fields[1]))
								}
							}

							assert.Assert(t, imageInUse, "the image row should be flagged as in use\n")
							// Only the platform the container actually runs is flagged, not every
							// platform of the image.
							assert.DeepEqual(t, platformsInUse, []string{hostPlatform})
						},
					}
				},
			},
			{
				Description: "conflicts with --quiet",
				Command:     test.Command("images", "--tree", "--quiet"),
				Expected: test.Expects(expect.ExitCodeGenericFail,
					[]error{errors.New("--quiet is not yet supported with --tree")}, nil),
			},
			{
				Description: "conflicts with --no-trunc",
				Command:     test.Command("images", "--tree", "--no-trunc"),
				Expected: test.Expects(expect.ExitCodeGenericFail,
					[]error{errors.New("--no-trunc is not yet supported with --tree")}, nil),
			},
			{
				Description: "conflicts with --format",
				Command:     test.Command("images", "--tree", "--format", "json"),
				Expected: test.Expects(expect.ExitCodeGenericFail,
					[]error{errors.New("--format is not yet supported with --tree")}, nil),
			},
			{
				Description: "conflicts with --digests",
				Command:     test.Command("images", "--tree", "--digests"),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					// docker names its internal flag in that message, nerdctl names the real one.
					message := "--digests is not yet supported with --tree"
					if nerdtest.IsDocker() {
						message = "--show-digest is not yet supported with --tree"
					}
					return test.Expects(expect.ExitCodeGenericFail, []error{errors.New(message)}, nil)(data, helpers)
				},
			},
			{
				Description: "conflicts with --names",
				// --names is a nerdctl-specific flag; Docker does not support it.
				Require: require.Not(nerdtest.Docker),
				Command: test.Command("images", "--tree", "--names"),
				Expected: test.Expects(expect.ExitCodeGenericFail,
					[]error{errors.New("--names is not yet supported with --tree")}, nil),
			},
		},
	}

	testCase.Run(t)
}
