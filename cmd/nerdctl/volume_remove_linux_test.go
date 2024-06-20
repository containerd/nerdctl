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
	"fmt"
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"gotest.tools/v3/icmd"
)

func TestVolumeRemove(t *testing.T) {
	t.Parallel()

	base := testutil.NewBase(t)

	malformed := "malformed volume name"
	notFound := "no such volume"
	requireArg := "requires at least 1 arg"
	inUse := "is in use"
	if base.Target == testutil.Docker {
		malformed = "no such volume"
	}

	testCases := []struct {
		description string
		command     func(tID string) *testutil.Cmd
		tearUp      func(tID string)
		tearDown    func(tID string)
		expected    func(tID string) icmd.Expected
	}{
		{
			description: "arg missing",
			command: func(tID string) *testutil.Cmd {
				return base.Cmd("volume", "rm")
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 1,
					Err:      requireArg,
				}
			},
		},
		{
			description: "invalid identifier",
			command: func(tID string) *testutil.Cmd {
				return base.Cmd("volume", "rm", "∞")
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 1,
					Err:      malformed,
				}
			},
		},
		{
			description: "non existent volume",
			command: func(tID string) *testutil.Cmd {
				return base.Cmd("volume", "rm", "doesnotexist")
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 1,
					Err:      notFound,
				}
			},
		},
		{
			description: "busy volume",
			command: func(tID string) *testutil.Cmd {
				return base.Cmd("volume", "rm", tID)
			},
			tearUp: func(tID string) {
				base.Cmd("volume", "create", tID).AssertOK()
				base.Cmd("run", "-v", fmt.Sprintf("%s:/volume", tID), "--name", tID, testutil.CommonImage).AssertOK()
			},
			tearDown: func(tID string) {
				base.Cmd("rm", "-f", tID).Run()
				base.Cmd("volume", "rm", "-f", tID).Run()
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 1,
					Err:      inUse,
				}

			},
		},
		{
			description: "freed volume",
			command: func(tID string) *testutil.Cmd {
				return base.Cmd("volume", "rm", tID)
			},
			tearUp: func(tID string) {
				base.Cmd("volume", "create", tID).AssertOK()
				base.Cmd("run", "-v", fmt.Sprintf("%s:/volume", tID), "--name", tID, testutil.CommonImage).AssertOK()
				base.Cmd("rm", "-f", tID).AssertOK()
			},
			tearDown: func(tID string) {
				base.Cmd("rm", "-f", tID).Run()
				base.Cmd("volume", "rm", "-f", tID).Run()
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					Out: tID,
				}
			},
		},
		{
			description: "dangling volume",
			command: func(tID string) *testutil.Cmd {
				return base.Cmd("volume", "rm", tID)
			},
			tearUp: func(tID string) {
				base.Cmd("volume", "create", tID).AssertOK()
			},
			tearDown: func(tID string) {
				base.Cmd("volume", "rm", "-f", tID).Run()
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					Out: tID,
				}
			},
		},
		{
			"part success multi remove",
			func(tID string) *testutil.Cmd {
				return base.Cmd("volume", "rm", "invalid∞", "nonexistent", tID)
			},
			func(tID string) {
				base.Cmd("volume", "create", tID).AssertOK()
			},
			func(tID string) {
				base.Cmd("volume", "rm", "-f", tID).Run()
			},
			func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 1,
					Out:      tID,
					Err:      notFound,
				}
			},
		},
		{
			description: "success multi-remove",
			command: func(tID string) *testutil.Cmd {
				return base.Cmd("volume", "rm", tID+"-1", tID+"-2")
			},
			tearUp: func(tID string) {
				base.Cmd("volume", "create", tID+"-1").AssertOK()
				base.Cmd("volume", "create", tID+"-2").AssertOK()
			},
			tearDown: func(tID string) {
				base.Cmd("volume", "rm", "-f", tID+"-1", tID+"-2").Run()
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					Out: tID + "-1\n" + tID + "-2",
				}
			},
		},
		{
			description: "failing multi-remove",
			command: func(tID string) *testutil.Cmd {
				return base.Cmd("volume", "rm", "nonexist1", "nonexist2")
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 1,
					Err:      notFound,
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
