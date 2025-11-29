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
	"errors"
	"fmt"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestNamespaceInspect(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Docker)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("namespace", "create", data.Identifier("first"))
		helpers.Ensure("namespace", "create", "--label", "foo=fooval", "--label", "bar=barval", data.Identifier("second"))

		// Create some resources in the namespaces
		helpers.Ensure("--namespace", data.Identifier("first"), "run", "-d", "--name", data.Identifier("container1"), testutil.CommonImage)
		helpers.Ensure("--namespace", data.Identifier("first"), "run", "-d", "--name", data.Identifier("container2"), testutil.CommonImage)
		helpers.Ensure("--namespace", data.Identifier("second"), "run", "-d", "--name", data.Identifier("container3"), testutil.CommonImage)
		// Create a volume
		helpers.Ensure("--namespace", data.Identifier("first"), "volume", "create", data.Identifier("volume1"))

		data.Labels().Set("ns1", data.Identifier("first"))
		data.Labels().Set("ns2", data.Identifier("second"))
		data.Labels().Set("container1", data.Identifier("container1"))
		data.Labels().Set("container2", data.Identifier("container2"))
		data.Labels().Set("container3", data.Identifier("container3"))
		data.Labels().Set("volume1", data.Identifier("volume1"))
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("--namespace", data.Identifier("first"), "image", "rm", "-f", testutil.CommonImage)
		helpers.Anyhow("--namespace", data.Identifier("second"), "image", "rm", "-f", testutil.CommonImage)
		helpers.Anyhow("--namespace", data.Identifier("first"), "rm", "-f", data.Identifier("container1"))
		helpers.Anyhow("--namespace", data.Identifier("first"), "rm", "-f", data.Identifier("container2"))
		helpers.Anyhow("--namespace", data.Identifier("second"), "rm", "-f", data.Identifier("container3"))
		helpers.Anyhow("--namespace", data.Identifier("first"), "volume", "rm", "-f", data.Identifier("volume1"))
		helpers.Anyhow("namespace", "remove", data.Identifier("first"))
		helpers.Anyhow("namespace", "remove", data.Identifier("second"))
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "arg missing should fail",
			Command:     test.Command("namespace", "inspect"),
			Expected:    test.Expects(1, []error{errors.New("requires at least 1 arg")}, nil),
		},
		{
			Description: "non existent namespace returns empty array",
			Command:     test.Command("namespace", "inspect", "doesnotexist"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.All(
						expect.JSON([]native.Namespace{}, func(dc []native.Namespace, t tig.T) {
							assert.Assert(t, len(dc) == 0, "expected empty array")
						}),
					),
				}
			},
		},
		{
			Description: "inspect labels",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("namespace", "inspect", data.Labels().Get("ns2"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.All(
						expect.Contains(data.Labels().Get("ns2")),
						expect.JSON([]native.Namespace{}, func(dc []native.Namespace, t tig.T) {
							labels := *dc[0].Labels
							assert.Assert(t, len(labels) == 2, fmt.Sprintf("two labels, not %d", len(labels)))
							assert.Assert(t, labels["foo"] == "fooval",
								fmt.Sprintf("label foo should be fooval, not %s", labels["foo"]))
							assert.Assert(t, labels["bar"] == "barval",
								fmt.Sprintf("label bar should be barval, not %s", labels["bar"]))
						}),
					),
				}
			},
		},
		{
			Description: "inspect details single namespace",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("namespace", "inspect", data.Labels().Get("ns1"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.All(
						expect.Contains(data.Labels().Get("ns1")),
						expect.JSON([]native.Namespace{}, func(dc []native.Namespace, t tig.T) {
							assert.Assert(t, len(dc[0].Volumes.Names) == 1, fmt.Sprintf("expected 1 volume name (was %d)", len(dc[0].Volumes.Names)))
							assert.Assert(t, dc[0].Containers.Count == 2, fmt.Sprintf("expected 2 container (was %d)", dc[0].Containers.Count))
							assert.Assert(t, len(dc[0].Images.IDs) == 1, fmt.Sprintf("expected 1 image (was %d)", dc[0].Images.Count))
						}),
					),
				}
			},
		},
		{
			Description: "inspect details both namespaces",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("namespace", "inspect", data.Labels().Get("ns1"), data.Labels().Get("ns2"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.All(
						expect.Contains(data.Labels().Get("ns1")),
						expect.JSON([]native.Namespace{}, func(dc []native.Namespace, t tig.T) {
							assert.Assert(t, len(dc[0].Volumes.Names) == 1, fmt.Sprintf("expected 1 volume (was %d)", len(dc[0].Volumes.Names)))
							assert.Assert(t, dc[0].Containers.Count == 2, fmt.Sprintf("expected 2 container (was %d)", dc[0].Containers.Count))
							assert.Assert(t, len(dc[0].Images.IDs) == 1, fmt.Sprintf("expected 1 image (was %d)", dc[0].Images.Count))
						}),

						expect.Contains(data.Labels().Get("ns2")),
						expect.JSON([]native.Namespace{}, func(dc []native.Namespace, t tig.T) {
							assert.Assert(t, len(dc[1].Volumes.Names) == 0, fmt.Sprintf("expected  0 volume (was %d)", len(dc[1].Volumes.Names)))
							assert.Assert(t, dc[1].Containers.Count == 1, fmt.Sprintf("expected 1 container (was %d)", dc[1].Containers.Count))
							assert.Assert(t, dc[1].Images.Count == 1, fmt.Sprintf("expected 1 image (was %d)", dc[1].Images.Count))
						}),
					),
				}
			},
		},
	}

	testCase.Run(t)
}
