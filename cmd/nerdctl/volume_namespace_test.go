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

package main

import (
	"testing"

	"gotest.tools/v3/icmd"

	"github.com/containerd/errdefs"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestVolumeNamespace(t *testing.T) {
	testutil.DockerIncompatible(t)

	t.Parallel()

	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)
	otherBase := testutil.NewBaseWithNamespace(t, tID+"-1")
	thirdBase := testutil.NewBaseWithNamespace(t, tID+"-2")

	tearUp := func(t *testing.T) {
		base.Cmd("volume", "create", tID).AssertOK()
	}

	tearDown := func(t *testing.T) {
		base.Cmd("volume", "rm", "-f", tID).Run()
		otherBase.Cmd("namespace", "rm", "-f", tID+"-1").Run()
		thirdBase.Cmd("namespace", "rm", "-f", tID+"-2").Run()
	}

	tearDown(t)
	t.Cleanup(func() {
		tearDown(t)
	})
	tearUp(t)

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
			description: "inspect another namespace volume should fail",
			command: func(tID string) *testutil.Cmd {
				return otherBase.Cmd("volume", "inspect", tID)
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 1,
					Err:      errdefs.ErrNotFound.Error(),
				}
			},
		},
		{
			description: "remove another namespace volume should fail",
			command: func(tID string) *testutil.Cmd {
				return otherBase.Cmd("volume", "remove", tID)
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 1,
					Err:      errdefs.ErrNotFound.Error(),
				}
			},
		},
		{
			description: "prune should leave other namespace untouched",
			command: func(tID string) *testutil.Cmd {
				return otherBase.Cmd("volume", "prune", "-a", "-f")
			},
			tearDown: func(tID string) {
				// Assert that the volume is here in the base namespace
				// both before and after the prune command
				base.Cmd("volume", "inspect", tID).AssertOK()
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 0,
				}
			},
		},
		{
			description: "create with namespace should work",
			command: func(tID string) *testutil.Cmd {
				return thirdBase.Cmd("volume", "create", tID)
			},
			tearDown: func(tID string) {
				thirdBase.Cmd("volume", "remove", "-f", tID).Run()
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
			if currentTest.dockerIncompatible {
				testutil.DockerIncompatible(tt)
			}

			tt.Parallel()

			// Note that here we are using the main test tID
			// since we are testing against the volume created in it
			if currentTest.tearDown != nil {
				currentTest.tearDown(tID)
				tt.Cleanup(func() {
					currentTest.tearDown(tID)
				})
			}
			if currentTest.tearUp != nil {
				currentTest.tearUp(tID)
			}

			// See https://github.com/containerd/nerdctl/issues/3130
			// We run first to capture the underlying icmd command and output
			cmd := currentTest.command(tID)
			res := cmd.Run()
			cmd.Assert(currentTest.expected(tID))
			if currentTest.inspect != nil {
				currentTest.inspect(tt, res.Stdout(), res.Stderr())
			}
		})
	}
}
