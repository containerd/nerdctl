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

package container

import (
	"bytes"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestStartDetachKeys(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Description: "TestStartDetachKeys",
		Require:     test.Binary("unbuffer"),
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("container", "rm", "-f", data.Identifier())
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			si := testutil.NewDelayOnceReader(strings.NewReader("exit\n"))

			cmd := helpers.
				Command("run", "-it", "--name", data.Identifier(), testutil.CommonImage)
			cmd.WithWrapper("unbuffer", "-p")
			cmd.WithStdin(si)
			cmd.Run(&test.Expected{
				Output: test.All(
					func(stdout string, info string, t *testing.T) {
						container := nerdtest.InspectContainer(helpers, data.Identifier())
						assert.Equal(t, container.State.Running, false, info)
					}),
			})
		},
		Command: func(data test.Data, helpers test.Helpers) test.Command {
			si := testutil.NewDelayOnceReader(bytes.NewReader([]byte{1, 2}))
			cmd := helpers.
				Command("start", "-a", "--detach-keys=ctrl-a,ctrl-b", data.Identifier())
			cmd.WithStdin(si)
			cmd.WithWrapper("unbuffer", "-p")
			return cmd
		},
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			return &test.Expected{
				Output: func(stdout string, info string, t *testing.T) {
					container := nerdtest.InspectContainer(helpers, data.Identifier())
					assert.Equal(t, container.State.Running, true, info)
				},
			}
		},
	}

	testCase.Run(t)

}
