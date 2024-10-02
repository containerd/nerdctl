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

func TestAttachDetachKeys(t *testing.T) {
	nerdtest.Setup()

	setup := func(data test.Data, helpers test.Helpers) {
		// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
		// unbuffer(1) can be installed with `apt-get install expect`.
		//
		// "-p" is needed because we need unbuffer to read from stdin, and from [1]:
		// "Normally, unbuffer does not read from stdin. This simplifies use of unbuffer in some situations.
		//  To use unbuffer in a pipeline, use the -p flag."
		//
		// [1] https://linux.die.net/man/1/unbuffer

		si := testutil.NewDelayOnceReader(bytes.NewReader(
			[]byte{16, 17}, // ctrl+p,ctrl+q, see https://www.physics.udel.edu/~watson/scen103/ascii.html
		))

		helpers.
			Command("run", "-it", "--name", data.Identifier(), testutil.CommonImage).
			WithWrapper("unbuffer", "-p").
			WithStdin(si).
			Run(&test.Expected{
				Output: test.All(
					// NOTE:
					// When detaching from a container, for a session started with 'docker attach',
					// it prints 'read escape sequence', but for one started with 'docker (run|start)', it prints nothing.
					// However, the flag is called '--detach-keys' in all cases, and nerdctl does print read detach keys
					// in all cases.
					// Disabling the contains test here allow both cli to run the test.
					// test.Contains("read detach keys"),
					func(stdout string, info string, t *testing.T) {
						container := nerdtest.InspectContainer(helpers, data.Identifier())
						assert.Equal(t, container.State.Running, true, info)
					}),
			})
	}

	testGroup := &test.Group{
		{
			Description: "TestAttachDefaultKeys",
			Require:     test.Binary("unbuffer"),
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("container", "rm", "-f", data.Identifier())
			},
			Setup: setup,
			Command: func(data test.Data, helpers test.Helpers) test.Command {
				si := testutil.NewDelayOnceReader(strings.NewReader("expr 1 + 1\nexit\n"))
				// `unbuffer -p` returns 0 even if the underlying nerdctl process returns a non-zero exit code,
				// so the exit code cannot be easily tested here.
				return helpers.
					Command("attach", data.Identifier()).
					WithStdin(si).
					WithWrapper("unbuffer", "-p")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						container := nerdtest.InspectContainer(helpers, data.Identifier())
						assert.Equal(t, container.State.Running, false, info)
					},
				}
			},
		},
		{
			Description: "TestAttachCustomKeys",
			Require:     test.Binary("unbuffer"),
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("container", "rm", "-f", data.Identifier())
			},
			Setup: setup,
			Command: func(data test.Data, helpers test.Helpers) test.Command {
				si := testutil.NewDelayOnceReader(bytes.NewReader([]byte{1, 2}))
				cmd := helpers.
					Command("attach", "--detach-keys=ctrl-a,ctrl-b", data.Identifier())
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
		},
	}

	testGroup.Run(t)
}
