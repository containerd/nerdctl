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

package volume

import (
	"testing"

	"gotest.tools/v3/icmd"

	"github.com/containerd/errdefs"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestVolumeCreate(t *testing.T) {
	t.Parallel()

	base := testutil.NewBase(t)

	malformed := errdefs.ErrInvalidArgument.Error()
	atMost := "at most 1 arg"
	exitCodeVariant := 1
	if base.Target == testutil.Docker {
		malformed = "invalid"
		exitCodeVariant = 125
	}

	testCases := []struct {
		description        string
		command            func(tID string) *testutil.Cmd
		tearUp             func(tID string)
		tearDown           func(tID string)
		expected           func(tID string) icmd.Expected
		inspect            func(t *testing.T, stdout string, stderr string)
		dockerIncompatible bool
	}{
		{
			description: "arg missing should create anonymous volume",
			command: func(tID string) *testutil.Cmd {
				return base.Cmd("volume", "create")
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 0,
				}
			},
		},
		{
			description: "invalid identifier should fail",
			command: func(tID string) *testutil.Cmd {
				return base.Cmd("volume", "create", "∞")
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 1,
					Err:      malformed,
				}
			},
		},
		{
			description: "too many args should fail",
			command: func(tID string) *testutil.Cmd {
				return base.Cmd("volume", "create", "too", "many")
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 1,
					Err:      atMost,
				}
			},
		},
		{
			description: "success",
			command: func(tID string) *testutil.Cmd {
				return base.Cmd("volume", "create", tID)
			},
			tearDown: func(tID string) {
				base.Cmd("volume", "rm", "-f", tID).Run()
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 0,
					Out:      tID,
				}
			},
		},
		{
			description: "success with labels",
			command: func(tID string) *testutil.Cmd {
				return base.Cmd("volume", "create", "--label", "foo1=baz1", "--label", "foo2=baz2", tID)
			},
			tearDown: func(tID string) {
				base.Cmd("volume", "rm", "-f", tID).Run()
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 0,
					Out:      tID,
				}
			},
		},
		{
			description: "invalid labels",
			command: func(tID string) *testutil.Cmd {
				// See https://github.com/containerd/nerdctl/issues/3126
				return base.Cmd("volume", "create", "--label", "a", "--label", "", tID)
			},
			tearDown: func(tID string) {
				base.Cmd("volume", "rm", "-f", tID).Run()
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: exitCodeVariant,
					Err:      malformed,
				}
			},
		},
		{
			description: "creating already existing volume should succeed",
			command: func(tID string) *testutil.Cmd {
				base.Cmd("volume", "create", tID).AssertOK()
				return base.Cmd("volume", "create", tID)
			},
			tearDown: func(tID string) {
				base.Cmd("volume", "rm", "-f", tID).Run()
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 0,
					Out:      tID,
				}
			},
		},
	}

	for _, test := range testCases {
		currentTest := test
		t.Run(currentTest.description, func(tt *testing.T) {
			tt.Parallel()

			tID := testutil.Identifier(tt)

			if currentTest.tearDown != nil {
				currentTest.tearDown(tID)
				tt.Cleanup(func() {
					currentTest.tearDown(tID)
				})
			}
			if currentTest.tearUp != nil {
				currentTest.tearUp(tID)
			}

			cmd := currentTest.command(tID)
			cmd.Assert(currentTest.expected(tID))
		})
	}
}
