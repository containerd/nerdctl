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

	"github.com/containerd/nerdctl/v2/pkg/tabutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestVolumeLsSize(t *testing.T) {
	nerdtest.Setup()

	tc := &test.Case{
		Description: "Volume ls --size",
		Require:     test.Not(nerdtest.Docker),
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("volume", "create", data.Identifier()+"-1")
			helpers.Ensure("volume", "create", data.Identifier()+"-2")
			helpers.Ensure("volume", "create", data.Identifier()+"-empty")
			vol1 := nerdtest.InspectVolume(helpers, data.Identifier()+"-1")
			vol2 := nerdtest.InspectVolume(helpers, data.Identifier()+"-2")

			err := createFileWithSize(vol1.Mountpoint, 102400)
			assert.NilError(t, err, "File creation failed")
			err = createFileWithSize(vol2.Mountpoint, 204800)
			assert.NilError(t, err, "File creation failed")
		},
		Command: test.RunCommand("volume", "ls", "--size"),
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			return &test.Expected{
				Output: func(stdout string, info string, t *testing.T) {
					var lines = strings.Split(strings.TrimSpace(stdout), "\n")
					assert.Assert(t, len(lines) >= 4, "expected at least 4 lines"+info)
					volSizes := map[string]string{
						data.Identifier() + "-1":     "100.0 KiB",
						data.Identifier() + "-2":     "200.0 KiB",
						data.Identifier() + "-empty": "0.0 B",
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
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("volume", "rm", "-f", data.Identifier()+"-1")
			helpers.Anyhow("volume", "rm", "-f", data.Identifier()+"-2")
			helpers.Anyhow("volume", "rm", "-f", data.Identifier()+"-empty")
		},
	}

	tc.Run(t)
}

func TestVolumeLsFilter(t *testing.T) {
	nerdtest.Setup()

	tc := &test.Case{
		Description: "Volume ls",
		Setup: func(data test.Data, helpers test.Helpers) {
			var vol1, vol2, vol3, vol4 = data.Identifier() + "-1", data.Identifier() + "-2", data.Identifier() + "-3", data.Identifier() + "-4"
			var label1, label2, label3, label4 = data.Identifier() + "=label-1", data.Identifier() + "=label-2", data.Identifier() + "=label-3", data.Identifier() + "-group=label-4"

			helpers.Ensure("volume", "create", "--label="+label1, "--label="+label4, vol1)
			helpers.Ensure("volume", "create", "--label="+label2, "--label="+label4, vol2)
			helpers.Ensure("volume", "create", "--label="+label3, vol3)
			helpers.Ensure("volume", "create", vol4)

			err := createFileWithSize(nerdtest.InspectVolume(helpers, vol1).Mountpoint, 409600)
			assert.NilError(t, err, "File creation failed")
			err = createFileWithSize(nerdtest.InspectVolume(helpers, vol2).Mountpoint, 1024000)
			assert.NilError(t, err, "File creation failed")
			err = createFileWithSize(nerdtest.InspectVolume(helpers, vol3).Mountpoint, 409600)
			assert.NilError(t, err, "File creation failed")
			err = createFileWithSize(nerdtest.InspectVolume(helpers, vol4).Mountpoint, 1024000)
			assert.NilError(t, err, "File creation failed")

			data.Set("vol1", vol1)
			data.Set("vol2", vol2)
			data.Set("vol3", vol3)
			data.Set("vol4", vol4)
			data.Set("mainlabel", data.Identifier())
			data.Set("label1", label1)
			data.Set("label2", label2)
			data.Set("label3", label3)
			data.Set("label4", label4)

		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("volume", "rm", "-f", data.Get("vol1"))
			helpers.Anyhow("volume", "rm", "-f", data.Get("vol2"))
			helpers.Anyhow("volume", "rm", "-f", data.Get("vol3"))
			helpers.Anyhow("volume", "rm", "-f", data.Get("vol4"))
		},
		SubTests: []*test.Case{
			{
				Description: "No filter",
				Command:     test.RunCommand("volume", "ls", "--quiet"),
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							var lines = strings.Split(strings.TrimSpace(stdout), "\n")
							assert.Assert(t, len(lines) >= 4, "expected at least 4 lines"+info)
							volNames := map[string]struct{}{
								data.Get("vol1"): {},
								data.Get("vol2"): {},
								data.Get("vol3"): {},
								data.Get("vol4"): {},
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
				Command: func(data test.Data, helpers test.Helpers) test.Command {
					return helpers.Command("volume", "ls", "--quiet", "--filter", "label="+data.Get("mainlabel"))
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							var lines = strings.Split(strings.TrimSpace(stdout), "\n")
							assert.Assert(t, len(lines) >= 3, "expected at least 3 lines"+info)
							volNames := map[string]struct{}{
								data.Get("vol1"): {},
								data.Get("vol2"): {},
								data.Get("vol3"): {},
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
				Command: func(data test.Data, helpers test.Helpers) test.Command {
					return helpers.Command("volume", "ls", "--quiet", "--filter", "label="+data.Get("label2"))
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							var lines = strings.Split(strings.TrimSpace(stdout), "\n")
							assert.Assert(t, len(lines) >= 1, "expected at least 1 lines"+info)
							volNames := map[string]struct{}{
								data.Get("vol2"): {},
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
				Command: func(data test.Data, helpers test.Helpers) test.Command {
					return helpers.Command("volume", "ls", "--quiet", "--filter", "label="+data.Get("mainlabel")+"=")
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
				Command: func(data test.Data, helpers test.Helpers) test.Command {
					return helpers.Command("volume", "ls", "--quiet", "--filter", "label="+data.Get("label1"), "--filter", "label="+data.Get("label2"))
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
				Command: func(data test.Data, helpers test.Helpers) test.Command {
					return helpers.Command("volume", "ls", "--quiet", "--filter", "label="+data.Get("mainlabel"), "--filter", "label="+data.Get("label4"))
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							var lines = strings.Split(strings.TrimSpace(stdout), "\n")
							assert.Assert(t, len(lines) >= 2, "expected at least 2 lines"+info)
							volNames := map[string]struct{}{
								data.Get("vol1"): {},
								data.Get("vol2"): {},
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
				Command: func(data test.Data, helpers test.Helpers) test.Command {
					return helpers.Command("volume", "ls", "--quiet", "--filter", "name="+data.Get("vol1"))
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							var lines = strings.Split(strings.TrimSpace(stdout), "\n")
							assert.Assert(t, len(lines) >= 1, "expected at least 1 line"+info)
							volNames := map[string]struct{}{
								data.Get("vol1"): {},
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
				// FIXME: https://github.com/containerd/nerdctl/issues/3452
				// Nerdctl filter behavior is broken
				Require: nerdtest.Docker,
				Command: func(data test.Data, helpers test.Helpers) test.Command {
					return helpers.Command("volume", "ls", "--quiet", "--filter", "name="+data.Get("vol1"), "--filter", "name="+data.Get("vol2"))
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							var lines = strings.Split(strings.TrimSpace(stdout), "\n")
							assert.Assert(t, len(lines) >= 2, "expected at least 2 lines"+info)
							volNames := map[string]struct{}{
								data.Get("vol1"): {},
								data.Get("vol2"): {},
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
				Require:     test.Not(nerdtest.Docker),
				Command: func(data test.Data, helpers test.Helpers) test.Command {
					return helpers.Command("volume", "ls", "--size", "--filter", "size=1024000")
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							var lines = strings.Split(strings.TrimSpace(stdout), "\n")
							assert.Assert(t, len(lines) >= 3, "expected at least 3 lines"+info)
							volNames := map[string]struct{}{
								data.Get("vol2"): {},
								data.Get("vol4"): {},
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
				Require:     test.Not(nerdtest.Docker),
				Command: func(data test.Data, helpers test.Helpers) test.Command {
					return helpers.Command("volume", "ls", "--size", "--filter", "size>=1024000", "--filter", "size<=2048000")
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							var lines = strings.Split(strings.TrimSpace(stdout), "\n")
							assert.Assert(t, len(lines) >= 3, "expected at least 3 lines"+info)
							volNames := map[string]struct{}{
								data.Get("vol2"): {},
								data.Get("vol4"): {},
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
				Require:     test.Not(nerdtest.Docker),
				Command: func(data test.Data, helpers test.Helpers) test.Command {
					return helpers.Command("volume", "ls", "--size", "--filter", "size>204800", "--filter", "size<1024000")
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							var lines = strings.Split(strings.TrimSpace(stdout), "\n")
							assert.Assert(t, len(lines) >= 3, "expected at least 3 lines"+info)
							volNames := map[string]struct{}{
								data.Get("vol1"): {},
								data.Get("vol3"): {},
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
		},
	}
	tc.Run(t)
}
