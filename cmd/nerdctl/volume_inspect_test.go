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
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/containerd/errdefs"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

func createFileWithSize(base *testutil.Base, vol string, size int64) {
	v := base.InspectVolume(vol)
	token := make([]byte, size)
	_, _ = rand.Read(token)
	err := os.WriteFile(filepath.Join(v.Mountpoint, "test-file"), token, 0644)
	assert.NilError(base.T, err)
}

func TestVolumeInspect(t *testing.T) {
	t.Parallel()

	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)

	var size int64 = 1028

	malformed := errdefs.ErrInvalidArgument.Error()
	notFound := errdefs.ErrNotFound.Error()
	requireArg := "requires at least 1 arg"
	if base.Target == testutil.Docker {
		malformed = "no such volume"
		notFound = "no such volume"
	}

	tearUp := func(t *testing.T) {
		base.Cmd("volume", "create", tID).AssertOK()
		base.Cmd("volume", "create", "--label", "foo=fooval", "--label", "bar=barval", tID+"-second").AssertOK()

		// Obviously note here that if inspect code gets totally hosed, this entire suite will
		// probably fail right here on the tearUp instead of actually testing something
		createFileWithSize(base, tID, size)
	}

	tearDown := func(t *testing.T) {
		base.Cmd("volume", "rm", "-f", tID).Run()
		base.Cmd("volume", "rm", "-f", tID+"-second").Run()
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
			description: "arg missing should fail",
			command: func(tID string) *testutil.Cmd {
				return base.Cmd("volume", "inspect")
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
				return base.Cmd("volume", "inspect", "∞")
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
				return base.Cmd("volume", "inspect", "doesnotexist")
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 1,
					Err:      notFound,
				}
			},
		},
		{
			description: "success",
			command: func(tID string) *testutil.Cmd {
				return base.Cmd("volume", "inspect", tID)
			},
			tearDown: func(tID string) {
				base.Cmd("volume", "rm", "-f", tID)
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 0,
					Out:      tID,
				}
			},
			inspect: func(t *testing.T, stdout string, stderr string) {
				var dc []native.Volume
				if err := json.Unmarshal([]byte(stdout), &dc); err != nil {
					t.Fatal(err)
				}
				assert.Assert(t, len(dc) == 1, fmt.Sprintf("one result, not %d", len(dc)))
				assert.Assert(t, dc[0].Name == tID, fmt.Sprintf("expected name to be %q (was %q)", tID, dc[0].Name))
				assert.Assert(t, dc[0].Labels == nil, fmt.Sprintf("expected labels to be nil and were %v", dc[0].Labels))
			},
		},
		{
			description: "inspect labels",
			command: func(tID string) *testutil.Cmd {
				return base.Cmd("volume", "inspect", tID+"-second")
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 0,
					Out:      tID,
				}
			},
			inspect: func(t *testing.T, stdout string, stderr string) {
				var dc []native.Volume
				if err := json.Unmarshal([]byte(stdout), &dc); err != nil {
					t.Fatal(err)
				}

				labels := *dc[0].Labels
				assert.Assert(t, len(labels) == 2, fmt.Sprintf("two results, not %d", len(labels)))
				assert.Assert(t, labels["foo"] == "fooval", fmt.Sprintf("label foo should be fooval, not %s", labels["foo"]))
				assert.Assert(t, labels["bar"] == "barval", fmt.Sprintf("label bar should be barval, not %s", labels["bar"]))
			},
		},
		{
			description: "inspect size",
			command: func(tID string) *testutil.Cmd {
				return base.Cmd("volume", "inspect", "--size", tID)
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 0,
					Out:      tID,
				}
			},
			inspect: func(t *testing.T, stdout string, stderr string) {
				var dc []native.Volume
				if err := json.Unmarshal([]byte(stdout), &dc); err != nil {
					t.Fatal(err)
				}
				assert.Assert(t, dc[0].Size == size, fmt.Sprintf("expected size to be %d (was %d)", size, dc[0].Size))
			},
			dockerIncompatible: true,
		},
		{
			description: "multi success",
			command: func(tID string) *testutil.Cmd {
				return base.Cmd("volume", "inspect", tID, tID+"-second")
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 0,
				}
			},
			inspect: func(t *testing.T, stdout string, stderr string) {
				var dc []native.Volume
				if err := json.Unmarshal([]byte(stdout), &dc); err != nil {
					t.Fatal(err)
				}
				assert.Assert(t, len(dc) == 2, fmt.Sprintf("two results, not %d", len(dc)))
				assert.Assert(t, dc[0].Name == tID, fmt.Sprintf("expected name to be %q (was %q)", tID, dc[0].Name))
				assert.Assert(t, dc[1].Name == tID+"-second", fmt.Sprintf("expected name to be %q (was %q)", tID+"-second", dc[1].Name))
			},
		},
		{
			description: "part success multi",
			command: func(tID string) *testutil.Cmd {
				return base.Cmd("volume", "inspect", "invalid∞", "nonexistent", tID)
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 1,
					Out:      tID,
					Err:      notFound,
				}
			},
			inspect: func(t *testing.T, stdout string, stderr string) {
				assert.Assert(t, strings.Contains(stderr, notFound))
				assert.Assert(t, strings.Contains(stderr, malformed))

				var dc []native.Volume
				if err := json.Unmarshal([]byte(stdout), &dc); err != nil {
					t.Fatal(err)
				}
				assert.Assert(t, len(dc) == 1, fmt.Sprintf("one result, not %d", len(dc)))
				assert.Assert(t, dc[0].Name == tID, fmt.Sprintf("expected name to be %q (was %q)", tID, dc[0].Name))
			},
		},
		{
			description: "multi failure",
			command: func(tID string) *testutil.Cmd {
				return base.Cmd("volume", "inspect", "invalid∞", "nonexistent")
			},
			expected: func(tID string) icmd.Expected {
				return icmd.Expected{
					ExitCode: 1,
				}
			},
			inspect: func(t *testing.T, stdout string, stderr string) {
				assert.Assert(t, strings.Contains(stderr, notFound))
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

			// We use the main test tID here
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
