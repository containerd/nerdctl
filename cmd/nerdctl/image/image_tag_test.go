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

	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/strutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestTagImageID(t *testing.T) {
	imageID := func(helpers test.Helpers, ref string) string {
		ids := strutil.DedupeStrSlice(strings.Fields(helpers.Capture("images", "--quiet", "--no-trunc", ref)))
		assert.Equal(helpers.T(), len(ids), 1)
		return strings.TrimPrefix(ids[0], "sha256:")
	}

	testCase := nerdtest.Setup()
	testCase.Require = nerdtest.Private
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("pull", "--quiet", testutil.CommonImage)
		data.Labels().Set("imageID", imageID(helpers, testutil.CommonImage))
	}

	for _, source := range []string{"name", "short ID", "full ID", "prefixed ID"} {
		testCase.SubTests = append(testCase.SubTests, &test.Case{
			Description: source,
			// The subtests tag the same image. Resolving an ID walks every name pointing at
			// that image, so a parallel subtest could resolve to a sibling's tag right
			// before that sibling removes it in its cleanup.
			NoParallel: true,
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rmi", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				id := data.Labels().Get("imageID")
				var ref string
				switch source {
				case "name":
					ref = testutil.CommonImage
				case "short ID":
					ref = id[:12]
				case "full ID":
					ref = id
				case "prefixed ID":
					ref = "sha256:" + id
				}
				return helpers.Command("tag", ref, data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, t tig.T) {
						assert.Equal(t, imageID(helpers, data.Identifier()), data.Labels().Get("imageID"))
					},
				}
			},
		})
	}
	testCase.Run(t)
}
