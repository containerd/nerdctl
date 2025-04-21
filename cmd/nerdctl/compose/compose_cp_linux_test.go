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

package compose

import (
	"fmt"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestComposeCopy(t *testing.T) {
	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

services:
  svc0:
    image: %s
    command: "sleep infinity"
`, testutil.CommonImage)

	const testFileContent = "test-file-content"

	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		compYamlPath := data.Temp().Save(dockerComposeYAML, "compose.yaml")
		helpers.Ensure("compose", "-f", compYamlPath, "up", "-d")

		srcFilePath := data.Temp().Save(testFileContent, "test-file")

		data.Labels().Set("composeYaml", compYamlPath)
		data.Labels().Set("srcFile", srcFilePath)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down", "-v")
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "test copy to service /dest-no-exist-no-slash",
			// These are expected to run in sequence
			NoParallel: true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose",
					"-f", data.Labels().Get("composeYaml"),
					"cp", data.Labels().Get("srcFile"), "svc0:/dest-no-exist-no-slash")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
		{
			Description: "test copy from service test-file2",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose",
					"-f", data.Labels().Get("composeYaml"),
					"cp", "svc0:/dest-no-exist-no-slash", data.Temp().Path("test-file2"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout, info string, t *testing.T) {
						copied := data.Temp().Load("test-file2")
						assert.Equal(t, copied, testFileContent)
					},
				}
			},
		},
	}

	testCase.Run(t)
}
