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

package inspect

import (
	"encoding/json"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestMain(m *testing.M) {
	testutil.M(m)
}

func TestInspectSimpleCase(t *testing.T) {
	nerdtest.Setup()
	testCase := &test.Case{
		Description: "inspect container and image return one single json array",
		Setup: func(data test.Data, helpers test.Helpers) {
			identifier := data.Identifier()
			helpers.Ensure("run", "-d", "--quiet", "--name", identifier, testutil.CommonImage, "sleep", nerdtest.Infinity)
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			identifier := data.Identifier()
			helpers.Anyhow("rm", "-f", identifier)
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("inspect", testutil.CommonImage, data.Identifier())
		},
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			return &test.Expected{
				Output: func(stdout string, info string, t *testing.T) {
					var inspectResult []json.RawMessage
					err := json.Unmarshal([]byte(stdout), &inspectResult)
					assert.NilError(t, err, "Unable to unmarshal output\n"+info)
					assert.Equal(t, len(inspectResult), 2, "Unexpectedly got multiple results\n"+info)

					var dci dockercompat.Image
					err = json.Unmarshal(inspectResult[0], &dci)
					assert.NilError(t, err, "Unable to unmarshal output\n"+info)
					inspecti := nerdtest.InspectImage(helpers, testutil.CommonImage)
					assert.Equal(t, dci.ID, inspecti.ID, info)

					var dcc dockercompat.Container
					err = json.Unmarshal(inspectResult[1], &dcc)
					assert.NilError(t, err, "Unable to unmarshal output\n"+info)
					inspectc := nerdtest.InspectContainer(helpers, data.Identifier())
					assert.Assert(t, dcc.ID == inspectc.ID, info)
				},
			}
		},
	}

	testCase.Run(t)
}
