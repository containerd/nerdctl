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
	"encoding/json"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestMain(m *testing.M) {
	testutil.M(m)
}

// TestNamespaceInspect verifies that `nerdctl namespace inspect <namespace-name>`
// returns correctly populated JSON for a namespace.
func TestNamespaceInspect(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Description: "namespace inspect returns populated metadata for a namespace",

		Setup: func(data test.Data, helpers test.Helpers) {
			ns := data.Identifier()

			helpers.Ensure("namespace", "create", ns)

			helpers.Ensure("--namespace", ns, "run", "-d", "--name", "test-cnt", testutil.CommonImage, "sleep", "3600")

			helpers.Ensure("--namespace", ns, "ps", "-a")
		},

		Cleanup: func(data test.Data, helpers test.Helpers) {
			ns := data.Identifier()
			helpers.Anyhow("--namespace", ns, "rm", "-f", "test-cnt")
			helpers.Anyhow("--namespace", ns, "rmi", "-f", testutil.CommonImage)
			helpers.Anyhow("namespace", "remove", ns)
		},

		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("namespace", "inspect", "--format", "json", data.Identifier())
		},

		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			return &test.Expected{
				Output: func(stdout string, t tig.T) {
					type NamespaceResult struct {
						Name       string `json:"Name"`
						Containers []struct {
							ID    string `json:"id"`
							Name  string `json:"name"`
							Image string `json:"image"`
						} `json:"Containers"`
					}

					var results []NamespaceResult

					err := json.Unmarshal([]byte(stdout), &results)
					if err != nil {
						var single NamespaceResult
						if errObj := json.Unmarshal([]byte(stdout), &single); errObj == nil {
							results = []NamespaceResult{single}
						} else {
							assert.NilError(t, err, "CLI output is neither a JSON array nor a valid object: %s", stdout)
						}
					}

					assert.Assert(t, len(results) > 0, "Expected at least one namespace result")

					var target *NamespaceResult
					for i := range results {
						if results[i].Name == data.Identifier() {
							target = &results[i]
							break
						}
					}

					assert.Assert(t, target != nil, "Namespace %s not found in results", data.Identifier())

					assert.Assert(t, len(target.Containers) > 0, "Containers list should not be empty for namespace %s", target.Name)

					c := target.Containers[0]
					assert.Assert(t, c.ID != "", "Container ID should not be empty")
					assert.Equal(t, c.Image, testutil.CommonImage, "Image mismatch")
				},
			}
		},
	}

	testCase.Run(t)
}
