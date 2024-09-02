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
	"fmt"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"

	"github.com/containerd/errdefs"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

// TestVolumeRemove does test a large variety of volume remove situations, albeit none of them being
// hard filesystem errors.
// Behavior in such cases is largely unspecified, as there is no easy way to compare with Docker.
// Anyhow, borked filesystem conditions is not something we should be expected to deal with in a smart way.
func TestVolumeRemove(t *testing.T) {
	t.Parallel()

	base := testutil.NewBase(t)

	inUse := errdefs.ErrFailedPrecondition.Error()
	malformed := errdefs.ErrInvalidArgument.Error()
	notFound := errdefs.ErrNotFound.Error()
	requireArg := "requires at least 1 arg"
	if base.Target == testutil.Docker {
		malformed = "no such volume"
		notFound = "no such volume"
		inUse = "volume is in use"
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
			description: "arg missing should fail",
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
			description: "invalid identifier should fail",
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
			description: "non existent volume should fail",
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
			description: "busy volume should fail",
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
			description: "busy anonymous volume should fail",
			command: func(tID string) *testutil.Cmd {
				// Inspect the container and find the anonymous volume id
				inspect := base.InspectContainer(tID)
				var anonName string
				for _, v := range inspect.Mounts {
					if v.Destination == "/volume" {
						anonName = v.Name
						break
					}
				}
				assert.Assert(t, anonName != "", "Failed to find anonymous volume id")

				// Try to remove that anon volume
				return base.Cmd("volume", "rm", anonName)
			},
			tearUp: func(tID string) {
				// base.Cmd("volume", "create", tID).AssertOK()
				base.Cmd("run", "-v", fmt.Sprintf("%s:/volume", tID), "--name", tID, testutil.CommonImage).AssertOK()
			},
			tearDown: func(tID string) {
				base.Cmd("rm", "-f", tID).Run()
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 1,
					Err:      inUse,
				}

			},
		},
		{
			description: "freed volume should succeed",
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
			description: "dangling volume should succeed",
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
			description: "part success multi-remove",
			command: func(tID string) *testutil.Cmd {
				return base.Cmd("volume", "rm", "invalid∞", "nonexistent", tID+"-busy", tID)
			},
			tearUp: func(tID string) {
				base.Cmd("volume", "create", tID).AssertOK()
				base.Cmd("volume", "create", tID+"-busy").AssertOK()
				base.Cmd("run", "-v", fmt.Sprintf("%s:/volume", tID+"-busy"), "--name", tID, testutil.CommonImage).AssertOK()
			},
			tearDown: func(tID string) {
				base.Cmd("rm", "-f", tID).Run()
				base.Cmd("volume", "rm", "-f", tID).Run()
				base.Cmd("volume", "rm", "-f", tID+"-busy").Run()
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 1,
					Out:      tID,
				}
			},
			inspect: func(t *testing.T, stdout string, stderr string) {
				assert.Assert(t, strings.Contains(stderr, notFound))
				assert.Assert(t, strings.Contains(stderr, inUse))
				assert.Assert(t, strings.Contains(stderr, malformed))
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
			tearUp: func(tID string) {
				base.Cmd("volume", "create", tID+"-busy").AssertOK()
				base.Cmd("run", "-v", fmt.Sprintf("%s:/volume", tID+"-busy"), "--name", tID, testutil.CommonImage).AssertOK()
			},
			tearDown: func(tID string) {
				base.Cmd("rm", "-f", tID).Run()
				base.Cmd("volume", "rm", "-f", tID+"-busy").Run()
			},
			command: func(tID string) *testutil.Cmd {
				return base.Cmd("volume", "rm", "invalid∞", "nonexistent", tID+"-busy")
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 1,
				}
			},
			inspect: func(t *testing.T, stdout string, stderr string) {
				assert.Assert(t, strings.Contains(stderr, notFound))
				assert.Assert(t, strings.Contains(stderr, inUse))
				assert.Assert(t, strings.Contains(stderr, malformed))
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
