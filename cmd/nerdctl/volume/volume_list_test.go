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
	"os"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/tabutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestVolumeLsSize(t *testing.T) {
	nerdtest.Setup()

	tc := &test.Case{
		Require: require.Not(nerdtest.Docker),
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("volume", "create", data.Identifier("1"))
			helpers.Ensure("volume", "create", data.Identifier("2"))
			helpers.Ensure("volume", "create", data.Identifier("empty"))
			vol1 := nerdtest.InspectVolume(helpers, data.Identifier("1"))
			vol2 := nerdtest.InspectVolume(helpers, data.Identifier("2"))

			err := createFileWithSize(vol1.Mountpoint, 102400)
			assert.NilError(t, err, "File creation failed")
			err = createFileWithSize(vol2.Mountpoint, 204800)
			assert.NilError(t, err, "File creation failed")
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("volume", "rm", "-f", data.Identifier("1"))
			helpers.Anyhow("volume", "rm", "-f", data.Identifier("2"))
			helpers.Anyhow("volume", "rm", "-f", data.Identifier("empty"))
		},
		Command: test.Command("volume", "ls", "--size"),
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			return &test.Expected{
				Output: func(stdout string, info string, t *testing.T) {
					var lines = strings.Split(strings.TrimSpace(stdout), "\n")
					assert.Assert(t, len(lines) >= 4, "expected at least 4 lines"+info)
					volSizes := map[string]string{
						data.Identifier("1"):     "100.0 KiB",
						data.Identifier("2"):     "200.0 KiB",
						data.Identifier("empty"): "0.0 B",
					}

					var numMatches = 0
					var tab = tabutil.NewReader("VOLUME NAME\tDIRECTORY\tSIZE")
					var err = tab.ParseHeader(lines[0])
					assert.NilError(t, err, info)

					for _, line := range lines {
						name, _ := tab.ReadRow(line, "VOLUME NAME")
						size, _ := tab.ReadRow(line, "SIZE")
						expectSize, ok := volSizes[name]
						if !ok {
							continue
						}
						assert.Assert(t, size == expectSize, fmt.Sprintf("expected size %s for volume %s, got %s", expectSize, name, size)+info)
						numMatches++
					}
					assert.Assert(t, numMatches == len(volSizes), fmt.Sprintf("expected %d volumes, got: %d", len(volSizes), numMatches)+info)
				},
			}
		},
	}

	tc.Run(t)
}

func TestVolumeLsFilter(t *testing.T) {
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
		var vol1, vol2, vol3, vol4 = data.Identifier("1"), data.Identifier("2"), data.Identifier("3"), data.Identifier("4")
		var label1, label2, label3, label4 = "mylabel=label-1", "mylabel=label-2", "mylabel=label-3", "mylabel-group=label-4"

		helpers.Ensure("volume", "create", "--label="+label1, "--label="+label4, vol1)
		helpers.Ensure("volume", "create", "--label="+label2, "--label="+label4, vol2)
		helpers.Ensure("volume", "create", "--label="+label3, vol3)
		helpers.Ensure("volume", "create", vol4)

		// FIXME
		// This will not work with Docker rootful and Docker cli run as a user
		// We should replace it with cp inside the container
		err := createFileWithSize(nerdtest.InspectVolume(helpers, vol1).Mountpoint, 409600)
		assert.NilError(t, err, "File creation failed")
		err = createFileWithSize(nerdtest.InspectVolume(helpers, vol2).Mountpoint, 1024000)
		assert.NilError(t, err, "File creation failed")
		err = createFileWithSize(nerdtest.InspectVolume(helpers, vol3).Mountpoint, 409600)
		assert.NilError(t, err, "File creation failed")
		err = createFileWithSize(nerdtest.InspectVolume(helpers, vol4).Mountpoint, 1024000)
		assert.NilError(t, err, "File creation failed")

		data.Labels().Set("vol1", vol1)
		data.Labels().Set("vol2", vol2)
		data.Labels().Set("vol3", vol3)
		data.Labels().Set("vol4", vol4)
		data.Labels().Set("mainlabel", "mylabel")
		data.Labels().Set("label1", label1)
		data.Labels().Set("label2", label2)
		data.Labels().Set("label3", label3)
		data.Labels().Set("label4", label4)

	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("volume", "rm", "-f", data.Labels().Get("vol1"))
		helpers.Anyhow("volume", "rm", "-f", data.Labels().Get("vol2"))
		helpers.Anyhow("volume", "rm", "-f", data.Labels().Get("vol3"))
		helpers.Anyhow("volume", "rm", "-f", data.Labels().Get("vol4"))
	}
	testCase.SubTests = []*test.Case{
		{
			Description: "No filter",
			Command:     test.Command("volume", "ls", "--quiet"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						var lines = strings.Split(strings.TrimSpace(stdout), "\n")
						assert.Assert(t, len(lines) >= 4, "expected at least 4 lines"+info)
						volNames := map[string]struct{}{
							data.Labels().Get("vol1"): {},
							data.Labels().Get("vol2"): {},
							data.Labels().Get("vol3"): {},
							data.Labels().Get("vol4"): {},
						}
						var numMatches = 0
						for _, name := range lines {
							_, ok := volNames[name]
							if !ok {
								continue
							}
							numMatches++
						}
						assert.Assert(t, len(volNames) == numMatches, fmt.Sprintf("expected %d volumes, got: %d", len(volNames), numMatches))
					},
				}
			},
		},
		{
			Description: "Retrieving label=mainlabel",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("volume", "ls", "--quiet", "--filter", "label="+data.Labels().Get("mainlabel"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						var lines = strings.Split(strings.TrimSpace(stdout), "\n")
						assert.Assert(t, len(lines) >= 3, "expected at least 3 lines"+info)
						volNames := map[string]struct{}{
							data.Labels().Get("vol1"): {},
							data.Labels().Get("vol2"): {},
							data.Labels().Get("vol3"): {},
						}
						for _, name := range lines {
							_, ok := volNames[name]
							assert.Assert(t, ok, fmt.Sprintf("unexpected volume %s found", name)+info)
						}
					},
				}
			},
		},
		{
			Description: "Retrieving label=mainlabel=label2",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("volume", "ls", "--quiet", "--filter", "label="+data.Labels().Get("label2"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						var lines = strings.Split(strings.TrimSpace(stdout), "\n")
						assert.Assert(t, len(lines) >= 1, "expected at least 1 lines"+info)
						volNames := map[string]struct{}{
							data.Labels().Get("vol2"): {},
						}
						for _, name := range lines {
							_, ok := volNames[name]
							assert.Assert(t, ok, fmt.Sprintf("unexpected volume %s found", name)+info)
						}
					},
				}
			},
		},
		{
			Description: "Retrieving label=mainlabel=",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("volume", "ls", "--quiet", "--filter", "label="+data.Labels().Get("mainlabel")+"=")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						assert.Assert(t, strings.TrimSpace(stdout) == "", "expected no result"+info)
					},
				}
			},
		},
		{
			Description: "Retrieving label=mainlabel=label1 and label=mainlabel=label2",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("volume", "ls", "--quiet", "--filter", "label="+data.Labels().Get("label1"), "--filter", "label="+data.Labels().Get("label2"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						assert.Assert(t, strings.TrimSpace(stdout) == "", "expected no result"+info)
					},
				}
			},
		},
		{
			Description: "Retrieving label=mainlabel and label=grouplabel=label4",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("volume", "ls", "--quiet", "--filter", "label="+data.Labels().Get("mainlabel"), "--filter", "label="+data.Labels().Get("label4"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						var lines = strings.Split(strings.TrimSpace(stdout), "\n")
						assert.Assert(t, len(lines) >= 2, "expected at least 2 lines"+info)
						volNames := map[string]struct{}{
							data.Labels().Get("vol1"): {},
							data.Labels().Get("vol2"): {},
						}
						for _, name := range lines {
							_, ok := volNames[name]
							assert.Assert(t, ok, fmt.Sprintf("unexpected volume %s found", name)+info)
						}
					},
				}
			},
		},
		{
			Description: "Retrieving name=volume1",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("volume", "ls", "--quiet", "--filter", "name="+data.Labels().Get("vol1"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						var lines = strings.Split(strings.TrimSpace(stdout), "\n")
						assert.Assert(t, len(lines) >= 1, "expected at least 1 line"+info)
						volNames := map[string]struct{}{
							data.Labels().Get("vol1"): {},
						}
						for _, name := range lines {
							_, ok := volNames[name]
							assert.Assert(t, ok, fmt.Sprintf("unexpected volume %s found", name)+info)
						}
					},
				}
			},
		},
		{
			Description: "Retrieving name=volume1 and name=volume2",
			// Nerdctl filter behavior is broken
			Require: nerdtest.NerdctlNeedsFixing("https://github.com/containerd/nerdctl/issues/3452"),
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("volume", "ls", "--quiet", "--filter", "name="+data.Labels().Get("vol1"), "--filter", "name="+data.Labels().Get("vol2"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						var lines = strings.Split(strings.TrimSpace(stdout), "\n")
						assert.Assert(t, len(lines) >= 2, "expected at least 2 lines"+info)
						volNames := map[string]struct{}{
							data.Labels().Get("vol1"): {},
							data.Labels().Get("vol2"): {},
						}
						for _, name := range lines {
							_, ok := volNames[name]
							assert.Assert(t, ok, fmt.Sprintf("unexpected volume %s found", name)+info)
						}
					},
				}
			},
		},
		{
			Description: "Retrieving size=1024000",
			Require:     require.Not(nerdtest.Docker),
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("volume", "ls", "--size", "--filter", "size=1024000")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						var lines = strings.Split(strings.TrimSpace(stdout), "\n")
						assert.Assert(t, len(lines) >= 3, "expected at least 3 lines"+info)
						volNames := map[string]struct{}{
							data.Labels().Get("vol2"): {},
							data.Labels().Get("vol4"): {},
						}
						var tab = tabutil.NewReader("VOLUME NAME\tDIRECTORY\tSIZE")
						var err = tab.ParseHeader(lines[0])
						assert.NilError(t, err, "Tab reader failed")
						for _, line := range lines {

							name, _ := tab.ReadRow(line, "VOLUME NAME")
							if name == "VOLUME NAME" {
								continue
							}
							_, ok := volNames[name]
							assert.Assert(t, ok, fmt.Sprintf("unexpected volume %s found", name)+info)
						}
					},
				}
			},
		},
		{
			Description: "Retrieving size>=1024000 size<=2048000",
			Require:     require.Not(nerdtest.Docker),
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("volume", "ls", "--size", "--filter", "size>=1024000", "--filter", "size<=2048000")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						var lines = strings.Split(strings.TrimSpace(stdout), "\n")
						assert.Assert(t, len(lines) >= 3, "expected at least 3 lines"+info)
						volNames := map[string]struct{}{
							data.Labels().Get("vol2"): {},
							data.Labels().Get("vol4"): {},
						}
						var tab = tabutil.NewReader("VOLUME NAME\tDIRECTORY\tSIZE")
						var err = tab.ParseHeader(lines[0])
						assert.NilError(t, err, "Tab reader failed")
						for _, line := range lines {

							name, _ := tab.ReadRow(line, "VOLUME NAME")
							if name == "VOLUME NAME" {
								continue
							}
							_, ok := volNames[name]
							assert.Assert(t, ok, fmt.Sprintf("unexpected volume %s found", name)+info)
						}
					},
				}
			},
		},
		{
			Description: "Retrieving size>204800 size<1024000",
			Require:     require.Not(nerdtest.Docker),
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("volume", "ls", "--size", "--filter", "size>204800", "--filter", "size<1024000")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						var lines = strings.Split(strings.TrimSpace(stdout), "\n")
						assert.Assert(t, len(lines) >= 3, "expected at least 3 lines"+info)
						volNames := map[string]struct{}{
							data.Labels().Get("vol1"): {},
							data.Labels().Get("vol3"): {},
						}
						var tab = tabutil.NewReader("VOLUME NAME\tDIRECTORY\tSIZE")
						var err = tab.ParseHeader(lines[0])
						assert.NilError(t, err, "Tab reader failed")
						for _, line := range lines {

							name, _ := tab.ReadRow(line, "VOLUME NAME")
							if name == "VOLUME NAME" {
								continue
							}
							_, ok := volNames[name]
							assert.Assert(t, ok, fmt.Sprintf("unexpected volume %s found", name)+info)
						}
					},
				}
			},
		},
	}

	testCase.Run(t)
}
