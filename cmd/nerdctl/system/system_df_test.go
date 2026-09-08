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

package system

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

// dfRow returns the columns of the `nerdctl system df` summary row of the given type. The type is
// matched as a prefix because "Local Volumes" contains a space.
func dfRow(t tig.T, stdout, rowType string) []string {
	for line := range strings.SplitSeq(stdout, "\n") {
		if columns, ok := strings.CutPrefix(line, rowType); ok {
			return strings.Fields(columns)
		}
	}
	t.Log(stdout)
	t.FailNow()
	return nil
}

// dfTotal and dfActive are the TOTAL and ACTIVE columns of a summary row.
func dfTotal(t tig.T, stdout, rowType string) string {
	return dfRow(t, stdout, rowType)[0]
}

func dfActive(t tig.T, stdout, rowType string) string {
	return dfRow(t, stdout, rowType)[1]
}

// baselineLabel holds the output of `system df` from before the test created anything.
const baselineLabel = "df-baseline"

// dfGrewBy asserts that a column of a summary row went up by n since the baseline. Only the
// difference a test makes can be asserted: `nerdtest.Private` gives nerdctl a namespace of its own,
// but docker has none, so its daemon still holds whatever the other tests left behind.
func dfGrewBy(t tig.T, data test.Data, stdout, rowType string, column, n int) {
	base := data.Labels().Get(baselineLabel)
	before, err := strconv.Atoi(dfRow(t, base, rowType)[column])
	assert.NilError(t, err, base)
	after, err := strconv.Atoi(dfRow(t, stdout, rowType)[column])
	assert.NilError(t, err, stdout)
	assert.Equal(t, after, before+n, stdout)
}

// The columns dfGrewBy counts, in the order `system df` prints them.
const (
	totalColumn = iota
	activeColumn
)

// dfReclaimable is the RECLAIMABLE column, which carries a percentage as a second field.
func dfReclaimable(t tig.T, stdout, rowType string) string {
	return strings.Join(dfRow(t, stdout, rowType)[3:], " ")
}

func TestSystemDf(t *testing.T) {
	testCase := nerdtest.Setup()

	// The counts are only meaningful when nothing else is running against the same namespace.
	testCase.NoParallel = true

	testCase.SubTests = []*test.Case{
		{
			Description: "empty namespace",
			// Docker has no namespaces, so there is no way to get a guaranteed empty daemon.
			Require: require.All(nerdtest.Private, require.Not(nerdtest.Docker)),
			Command: test.Command("system", "df"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, t tig.T) {
						assert.Assert(t, strings.Contains(stdout, "TYPE"), stdout)
						for _, rowType := range []string{"Images", "Containers", "Local Volumes"} {
							assert.Equal(t, dfTotal(t, stdout, rowType), "0", stdout)
							assert.Equal(t, dfActive(t, stdout, rowType), "0", stdout)
						}
						// The build cache is not namespaced, so it is not asserted on here.
						assert.Assert(t, strings.Contains(stdout, "Build Cache"), stdout)
					},
				}
			},
		},
		{
			Description: "running container",
			Require:     nerdtest.Private,
			Setup: func(data test.Data, helpers test.Helpers) {
				data.Labels().Set(baselineLabel, helpers.Capture("system", "df"))
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					testutil.CommonImage, "sleep", nerdtest.Infinity)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: test.Command("system", "df"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, t tig.T) {
						// The container is created by this test, so it is the one the counts went
						// up by.
						dfGrewBy(t, data, stdout, "Containers", totalColumn, 1)
						dfGrewBy(t, data, stdout, "Containers", activeColumn, 1)

						// The image the container runs is in use, so it is active and nothing of
						// it can be reclaimed. Neither holds as a difference: the image may well
						// have been pulled and in use already, which is what a shared daemon
						// cannot be asked about.
						if !nerdtest.IsDocker() {
							assert.Equal(t, dfActive(t, stdout, "Images"), "1", stdout)
							assert.Equal(t, dfReclaimable(t, stdout, "Images"), "0B (0%)", stdout)
						}
					},
				}
			},
		},
		{
			Description: "stopped container is reclaimable",
			Require:     nerdtest.Private,
			Setup: func(data test.Data, helpers test.Helpers) {
				data.Labels().Set(baselineLabel, helpers.Capture("system", "df"))
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					testutil.CommonImage, "sleep", nerdtest.Infinity)
				helpers.Ensure("stop", data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: test.Command("system", "df"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, t tig.T) {
						// A stopped container is still counted, it just is not active any more,
						// so its space can be reclaimed.
						dfGrewBy(t, data, stdout, "Containers", totalColumn, 1)
						dfGrewBy(t, data, stdout, "Containers", activeColumn, 0)

						// The image is no longer held by a running container, but it is still
						// referenced by it, so it stays active.
						if !nerdtest.IsDocker() {
							assert.Equal(t, dfActive(t, stdout, "Images"), "1", stdout)
						}
					},
				}
			},
		},
		{
			Description: "unused image is reclaimable",
			Require:     require.All(nerdtest.Private, require.Not(nerdtest.Docker)),
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("pull", "--quiet", testutil.CommonImage)
			},
			Command: test.Command("system", "df"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, t tig.T) {
						assert.Equal(t, dfTotal(t, stdout, "Images"), "1", stdout)
						assert.Equal(t, dfActive(t, stdout, "Images"), "0", stdout)
						// Nothing else holds the layers, so practically the whole image can be
						// reclaimed. It falls just short of the total rather than matching it,
						// because the index listing the manifests is on disk, and Docker counts
						// it in the total while charging no single image for it.
						_, percent, ok := strings.Cut(dfReclaimable(t, stdout, "Images"), " ")
						assert.Assert(t, ok, stdout)
						value, err := strconv.Atoi(strings.Trim(percent, "(%)"))
						assert.NilError(t, err, stdout)
						assert.Assert(t, value >= 99, stdout)
					},
				}
			},
		},
		{
			Description: "verbose",
			Require:     nerdtest.Private,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(),
					testutil.CommonImage, "sleep", nerdtest.Infinity)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: test.Command("system", "df", "--verbose"),
			Expected: test.Expects(0, nil, expect.All(
				expect.Contains("Images space usage:"),
				expect.Contains("SHARED SIZE"),
				expect.Contains("UNIQUE SIZE"),
				expect.Contains("Containers space usage:"),
				expect.Contains("LOCAL VOLUMES"),
				expect.Contains("Local Volumes space usage:"),
				expect.Contains("LINKS"),
				expect.Contains("Build cache usage:"),
			)),
		},
		{
			Description: "format json",
			Require:     nerdtest.Private,
			Command:     test.Command("system", "df", "--format", "json"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, t tig.T) {
						var types []string
						for line := range strings.SplitSeq(strings.TrimSpace(stdout), "\n") {
							row := map[string]string{}
							assert.NilError(t, json.Unmarshal([]byte(line), &row), line)
							types = append(types, row["Type"])
						}
						assert.DeepEqual(t, types,
							[]string{"Images", "Containers", "Local Volumes", "Build Cache"})
					},
				}
			},
		},
		{
			Description: "format template",
			Require:     nerdtest.Private,
			Command:     test.Command("system", "df", "--format", "{{.Type}}"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output:   expect.Equals("Images\nContainers\nLocal Volumes\nBuild Cache\n"),
				}
			},
		},
		{
			Description: "format table template",
			Require:     nerdtest.Private,
			Command:     test.Command("system", "df", "--format", `table {{.Type}}\t{{.Size}}`),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, t tig.T) {
						lines := strings.Split(strings.TrimSpace(stdout), "\n")
						assert.Equal(t, len(lines), 5, stdout)
						// The header names only the requested columns, and the \t the shell passed
						// through literally became a real column separator.
						assert.Equal(t, strings.Join(strings.Fields(lines[0]), " "), "TYPE SIZE", stdout)
						assert.Equal(t, strings.Fields(lines[1])[0], "Images", stdout)
					},
				}
			},
		},
	}

	testCase.Run(t)
}
