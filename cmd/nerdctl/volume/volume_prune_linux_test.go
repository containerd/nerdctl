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
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestVolumePrune(t *testing.T) {
	// Volume pruning cannot be parallelized for Docker, since we need namespaces to do that in a way that does interact with other tests
	if testutil.GetTarget() != testutil.Docker {
		t.Parallel()
	}

	// FIXME: for an unknown reason, when testing ipv6, calling NewBaseWithNamespace per sub-test, in the tearDown/tearUp methods
	// will actually panic the test (also happens with target=docker)
	// Calling base here *first* so that it can skip NOW - does seem to workaround the problem
	// If you have any idea how to properly do this, feel free to remove the following line and fix the underlying issue
	testutil.NewBase(t)

	subTearUp := func(tID string) {
		base := testutil.NewBaseWithNamespace(t, tID)
		res := base.Cmd("volume", "create").Run()
		anonIDBusy := res.Stdout()
		base.Cmd("volume", "create").Run()
		base.Cmd("volume", "create", tID+"-busy").AssertOK()
		base.Cmd("volume", "create", tID+"-free").AssertOK()
		base.Cmd("run", "--name", tID,
			"-v", tID+"-busy:/whatever",
			"-v", anonIDBusy, testutil.CommonImage).AssertOK()
	}

	subTearDown := func(tID string) {
		base := testutil.NewBaseWithNamespace(t, tID)
		base.Cmd("rm", "-f", tID).Run()
		base.Cmd("namespace", "remove", "-f", tID).Run()
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
			description: "prune anonymous only",
			command: func(tID string) *testutil.Cmd {
				base := testutil.NewBaseWithNamespace(t, tID)
				return base.Cmd("volume", "prune", "-f")
			},
			tearUp:   subTearUp,
			tearDown: subTearDown,
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 0,
				}
			},
			inspect: func(t *testing.T, stdout string, stderr string) {
				tID := testutil.Identifier(t)
				base := testutil.NewBaseWithNamespace(t, tID)
				assert.Assert(base.T, !strings.Contains(stdout, tID+"-free"))
				base.Cmd("volume", "inspect", tID+"-free").AssertOK()
				assert.Assert(base.T, !strings.Contains(stdout, tID+"-busy"))
				base.Cmd("volume", "inspect", tID+"-busy").AssertOK()
				// TODO verify the anonymous volumes status
			},
		},
		{
			description: "prune all",
			command: func(tID string) *testutil.Cmd {
				base := testutil.NewBaseWithNamespace(t, tID)
				return base.Cmd("volume", "prune", "-f", "--all")
			},
			tearUp:   subTearUp,
			tearDown: subTearDown,
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 0,
				}
			},
			inspect: func(t *testing.T, stdout string, stderr string) {
				tID := testutil.Identifier(t)
				base := testutil.NewBaseWithNamespace(t, tID)
				assert.Assert(t, !strings.Contains(stdout, tID+"-busy"))
				base.Cmd("volume", "inspect", tID+"-busy").AssertOK()
				assert.Assert(t, strings.Contains(stdout, tID+"-free"))
				base.Cmd("volume", "inspect", tID+"-free").AssertFail()
				// TODO verify the anonymous volumes status
			},
		},
	}

	for _, test := range testCases {
		currentTest := test
		t.Run(currentTest.description, func(tt *testing.T) {
			if currentTest.dockerIncompatible {
				testutil.DockerIncompatible(tt)
			}

			if testutil.GetTarget() != testutil.Docker {
				tt.Parallel()
			}

			subTID := testutil.Identifier(tt)

			if currentTest.tearDown != nil {
				currentTest.tearDown(subTID)
				tt.Cleanup(func() {
					currentTest.tearDown(subTID)
				})
			}
			if currentTest.tearUp != nil {
				currentTest.tearUp(subTID)
			}

			// See https://github.com/containerd/nerdctl/issues/3130
			// We run first to capture the underlying icmd command and output
			cmd := currentTest.command(subTID)
			res := cmd.Run()
			cmd.Assert(currentTest.expected(subTID))
			if currentTest.inspect != nil {
				currentTest.inspect(tt, res.Stdout(), res.Stderr())
			}
		})
	}
}
