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

package manifest

import (
	"errors"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestManifestsRemove(t *testing.T) {
	testCase := nerdtest.Setup()
	manifestListName1 := "example.com/test-list-remove:v1"
	manifestListName2 := "example.com/test-list-remove:v2"
	manifestRef1 := testutil.GetTestImageWithoutTag("alpine") + "@" + testutil.GetTestImageManifestDigest("alpine", "linux/amd64")
	manifestRef2 := testutil.GetTestImageWithoutTag("alpine") + "@" + testutil.GetTestImageManifestDigest("alpine", "linux/arm64")

	testCase.SubTests = []*test.Case{
		{
			Description: "remove-several-manifestlists",
			Setup: func(data test.Data, helpers test.Helpers) {
				cmd := helpers.Command("manifest", "create", manifestListName1, manifestRef1)
				cmd.Run(&test.Expected{ExitCode: 0})
				cmd = helpers.Command("manifest", "create", manifestListName2, manifestRef2)
				cmd.Run(&test.Expected{ExitCode: 0})
			},
			Command: test.Command("manifest", "rm", manifestListName1, manifestListName2),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
				}
			},
		},
		{
			Description: "remove-non-existent-manifestlist",
			Command:     test.Command("manifest", "rm", "example.com/non-existent:latest"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New(data.Labels().Get("error"))},
				}
			},
			Data: test.WithLabels(map[string]string{
				"error": "No such manifest: example.com/non-existent:latest",
			}),
		},
	}

	testCase.Run(t)
}
