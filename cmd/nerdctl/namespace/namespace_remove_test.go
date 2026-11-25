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

package namespace

import (
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestNamespaceRemoveMultiple(t *testing.T) {
	for _, tc := range []struct {
		description string
		firstExists bool
	}{
		{description: "both namespaces exist", firstExists: true},
		{description: "first namespace does not exist", firstExists: false},
	} {
		t.Run(tc.description, func(t *testing.T) {
			testCase := nerdtest.Setup()
			testCase.Require = require.Not(nerdtest.Docker)
			testCase.Setup = func(data test.Data, helpers test.Helpers) {
				if tc.firstExists {
					helpers.Ensure("namespace", "create", data.Identifier("first"))
				}
				helpers.Ensure("namespace", "create", data.Identifier("second"))
			}
			testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("namespace", "rm", data.Identifier("first"))
				helpers.Anyhow("namespace", "rm", data.Identifier("second"))
			}
			testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("namespace", "rm", data.Identifier("first"), data.Identifier("second"))
			}
			testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{Output: expect.Equals(data.Identifier("first") + "\n" + data.Identifier("second") + "\n")}
			}
			testCase.Run(t)
		})
	}
}
