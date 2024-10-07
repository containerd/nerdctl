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
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/errdefs"

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func createFileWithSize(mountPoint string, size int64) error {
	token := make([]byte, size)
	_, _ = rand.Read(token)
	err := os.WriteFile(filepath.Join(mountPoint, "test-file"), token, 0644)
	return err
}

func TestVolumeInspect(t *testing.T) {
	var size int64 = 1028

	testCase := nerdtest.Setup()

	testCase.Require = nerdtest.BrokenTest("This test assumes that the host-side of a volume can be written into, "+
		"which is not always true. To be replaced by cp into the container.",
		&test.Requirement{
			Check: func(data test.Data, helpers test.Helpers) (bool, string) {
				isDocker, _ := nerdtest.Docker.Check(data, helpers)
				return !isDocker || os.Geteuid() == 0, "docker cli needs to be run as root"
			},
		})

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("volume", "create", data.Identifier("first"))
		helpers.Ensure("volume", "create", "--label", "foo=fooval", "--label", "bar=barval", data.Identifier("second"))
		// Obviously note here that if inspect code gets totally hosed, this entire suite will
		// probably fail right here on the Setup instead of actually testing something
		vol := nerdtest.InspectVolume(helpers, data.Identifier("first"))
		err := createFileWithSize(vol.Mountpoint, size)
		assert.NilError(t, err, "File creation failed")
		data.Set("vol1", data.Identifier("first"))
		data.Set("vol2", data.Identifier("second"))
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("volume", "rm", "-f", data.Identifier("first"))
		helpers.Anyhow("volume", "rm", "-f", data.Identifier("second"))
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "arg missing should fail",
			Command:     test.Command("volume", "inspect"),
			Expected:    test.Expects(1, []error{errors.New("requires at least 1 arg")}, nil),
		},
		{
			Description: "invalid identifier should fail",
			Command:     test.Command("volume", "inspect", "∞"),
			Expected:    test.Expects(1, []error{errdefs.ErrInvalidArgument}, nil),
		},
		{
			Description: "non existent volume should fail",
			Command:     test.Command("volume", "inspect", "doesnotexist"),
			Expected:    test.Expects(1, []error{errdefs.ErrNotFound}, nil),
		},
		{
			Description: "success",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("volume", "inspect", data.Get("vol1"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: test.All(
						test.Contains(data.Get("vol1")),
						func(stdout string, info string, t *testing.T) {
							var dc []native.Volume
							if err := json.Unmarshal([]byte(stdout), &dc); err != nil {
								t.Fatal(err)
							}
							assert.Assert(t, len(dc) == 1, fmt.Sprintf("one result, not %d", len(dc))+info)
							assert.Assert(t, dc[0].Name == data.Get("vol1"), fmt.Sprintf("expected name to be %q (was %q)", data.Get("vol1"), dc[0].Name)+info)
							assert.Assert(t, dc[0].Labels == nil, fmt.Sprintf("expected labels to be nil and were %v", dc[0].Labels)+info)
						},
					),
				}
			},
		},
		{
			Description: "inspect labels",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("volume", "inspect", data.Get("vol2"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: test.All(
						test.Contains(data.Get("vol2")),
						func(stdout string, info string, t *testing.T) {
							var dc []native.Volume
							if err := json.Unmarshal([]byte(stdout), &dc); err != nil {
								t.Fatal(err)
							}
							labels := *dc[0].Labels
							assert.Assert(t, len(labels) == 2, fmt.Sprintf("two results, not %d", len(labels)))
							assert.Assert(t, labels["foo"] == "fooval", fmt.Sprintf("label foo should be fooval, not %s", labels["foo"]))
							assert.Assert(t, labels["bar"] == "barval", fmt.Sprintf("label bar should be barval, not %s", labels["bar"]))
						},
					),
				}
			},
		},
		{
			Description: "inspect size",
			Require:     test.Not(nerdtest.Docker),
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("volume", "inspect", "--size", data.Get("vol1"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: test.All(
						test.Contains(data.Get("vol1")),
						func(stdout string, info string, t *testing.T) {
							var dc []native.Volume
							if err := json.Unmarshal([]byte(stdout), &dc); err != nil {
								t.Fatal(err)
							}
							assert.Assert(t, dc[0].Size == size, fmt.Sprintf("expected size to be %d (was %d)", size, dc[0].Size))
						},
					),
				}
			},
		},
		{
			Description: "multi success",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("volume", "inspect", data.Get("vol1"), data.Get("vol2"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: test.All(
						test.Contains(data.Get("vol1")),
						test.Contains(data.Get("vol2")),
						func(stdout string, info string, t *testing.T) {
							var dc []native.Volume
							if err := json.Unmarshal([]byte(stdout), &dc); err != nil {
								t.Fatal(err)
							}
							assert.Assert(t, len(dc) == 2, fmt.Sprintf("two results, not %d", len(dc)))
							assert.Assert(t, dc[0].Name == data.Get("vol1"), fmt.Sprintf("expected name to be %q (was %q)", data.Get("vol1"), dc[0].Name))
							assert.Assert(t, dc[1].Name == data.Get("vol2"), fmt.Sprintf("expected name to be %q (was %q)", data.Get("vol2"), dc[1].Name))
						},
					),
				}
			},
		},
		{
			Description: "part success multi",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("volume", "inspect", "invalid∞", "nonexistent", data.Get("vol1"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errdefs.ErrNotFound, errdefs.ErrInvalidArgument},
					Output: test.All(
						test.Contains(data.Get("vol1")),
						func(stdout string, info string, t *testing.T) {
							var dc []native.Volume
							if err := json.Unmarshal([]byte(stdout), &dc); err != nil {
								t.Fatal(err)
							}
							assert.Assert(t, len(dc) == 1, fmt.Sprintf("one result, not %d", len(dc)))
							assert.Assert(t, dc[0].Name == data.Get("vol1"), fmt.Sprintf("expected name to be %q (was %q)", data.Get("vol1"), dc[0].Name))
						},
					),
				}
			},
		},
		{
			Description: "multi failure",
			Command:     test.Command("volume", "inspect", "invalid∞", "nonexistent"),
			Expected:    test.Expects(1, []error{errdefs.ErrNotFound, errdefs.ErrInvalidArgument}, nil),
		},
	}

	testCase.Run(t)
}
