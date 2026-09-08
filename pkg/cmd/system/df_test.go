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
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
)

func TestHumanReclaimable(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		reclaimable int64
		totalSize   int64
		expected    string
	}{
		{
			name:        "share of the total",
			reclaimable: 940,
			totalSize:   1000,
			expected:    "940B (94%)",
		},
		{
			name:        "nothing reclaimable",
			reclaimable: 0,
			totalSize:   1000,
			expected:    "0B (0%)",
		},
		{
			name:        "no total to compare against",
			reclaimable: 0,
			totalSize:   0,
			expected:    "0B",
		},
		{
			name:        "everything reclaimable",
			reclaimable: 2000,
			totalSize:   2000,
			expected:    "2kB (100%)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, humanReclaimable(tc.reclaimable, tc.totalSize), tc.expected)
		})
	}
}

func TestTruncateID(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		id       string
		expected string
	}{
		{
			name:     "digest",
			id:       "sha256:09538a1f51d3ec5af0449a1640937dfdf79b0e9b8c4da5b8a883086d5c1492ef",
			expected: "09538a1f51d3",
		},
		{
			name:     "opaque buildkit id",
			id:       "n3vkjqf4tzxkgxwjdgm0e5vpm",
			expected: "n3vkjqf4tzxk",
		},
		{
			name:     "shorter than the short id length",
			id:       "abc",
			expected: "abc",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, truncateID(tc.id), tc.expected)
		})
	}
}

// fieldsOfRowContaining returns the single-space-joined fields of the first line of out holding
// needle, so that assertions do not depend on how the tabwriter pads the columns.
func fieldsOfRowContaining(out, needle string) string {
	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(line, needle) {
			return strings.Join(strings.Fields(line), " ")
		}
	}
	return ""
}

func testDiskUsage() types.DiskUsage {
	createdAt := time.Now().Add(-time.Hour)
	lastUsedAt := time.Now().Add(-time.Minute)

	return types.DiskUsage{
		Images: types.ImageDiskUsage{
			TotalCount:  2,
			ActiveCount: 1,
			TotalSize:   1000,
			Reclaimable: 400,
			Items: []types.ImageDiskUsageItem{
				{
					ID:         "sha256:09538a1f51d3ec5af0449a1640937dfdf79b0e9b8c4da5b8a883086d5c1492ef",
					Repository: "example.com/foo",
					Tag:        "latest",
					CreatedAt:  createdAt,
					Size:       800,
					SharedSize: 200,
					Containers: 1,
				},
				{
					ID:        "sha256:0168606be2317b0d6a3c0b1a2a5b1a2a5b1a2a5b1a2a5b1a2a5b1a2a5b1a2a5b",
					CreatedAt: createdAt,
					Size:      600,
					// A dangling image with no container: everything unique to it is reclaimable.
					SharedSize: 200,
				},
			},
		},
		Containers: types.ContainerDiskUsage{
			TotalCount:  1,
			ActiveCount: 0,
			TotalSize:   100,
			Reclaimable: 100,
			Items: []types.ContainerDiskUsageItem{
				{
					ID:           "6d3f1c1c1a5b6d3f1c1c1a5b6d3f1c1c1a5b6d3f1c1c1a5b6d3f1c1c1a5b6d3f",
					Image:        "example.com/foo:latest",
					Command:      `"sleep 3600"`,
					LocalVolumes: 1,
					SizeRw:       100,
					CreatedAt:    createdAt,
					Status:       "Exited (0) 1 minute ago",
					Names:        "sleeper",
				},
			},
		},
		Volumes: types.VolumeDiskUsage{
			TotalCount:  2,
			ActiveCount: 1,
			TotalSize:   300,
			Reclaimable: 100,
			Items: []types.VolumeDiskUsageItem{
				{Name: "data", Links: 1, Size: 200},
				{Name: "orphan", Links: 0, Size: 100},
			},
		},
		BuildCache: types.BuildCacheDiskUsage{
			TotalCount:  1,
			ActiveCount: 0,
			TotalSize:   500,
			Reclaimable: 500,
			Items: []types.BuildCacheDiskUsageItem{
				{
					ID:         "n3vkjqf4tzxkgxwjdgm0e5vpm",
					CacheType:  "regular",
					Size:       500,
					CreatedAt:  createdAt,
					LastUsedAt: &lastUsedAt,
					UsageCount: 3,
				},
			},
		},
	}
}

func TestPrintDiskUsageSummary(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	err := printDiskUsage(testDiskUsage(), types.SystemDfOptions{Stdout: stdout})
	assert.NilError(t, err)

	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	assert.Equal(t, len(lines), 5)

	assert.Assert(t, strings.HasPrefix(lines[0], "TYPE"), lines[0])
	assert.Assert(t, strings.Contains(lines[0], "RECLAIMABLE"), lines[0])

	assert.Equal(t, strings.Join(strings.Fields(lines[1]), " "), "Images 2 1 1kB 400B (40%)")
	assert.Equal(t, strings.Join(strings.Fields(lines[2]), " "), "Containers 1 0 100B 100B (100%)")
	assert.Equal(t, strings.Join(strings.Fields(lines[3]), " "), "Local Volumes 2 1 300B 100B (33%)")
	// The build cache never gets a percentage, matching Docker.
	assert.Equal(t, strings.Join(strings.Fields(lines[4]), " "), "Build Cache 1 0 500B 500B")
}

func TestPrintDiskUsageEmpty(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	err := printDiskUsage(types.DiskUsage{}, types.SystemDfOptions{Stdout: stdout})
	assert.NilError(t, err)

	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	assert.Equal(t, len(lines), 5)
	for _, line := range lines[1:] {
		// Without a total there is nothing to take a percentage of.
		assert.Assert(t, strings.HasSuffix(line, "0B"), line)
	}
}

func TestPrintDiskUsageFormat(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	err := printDiskUsage(testDiskUsage(), types.SystemDfOptions{
		Stdout: stdout,
		Format: "{{.Type}}={{.Size}}",
	})
	assert.NilError(t, err)

	assert.Equal(t, stdout.String(), strings.Join([]string{
		"Images=1kB",
		"Containers=100B",
		"Local Volumes=300B",
		"Build Cache=500B",
		"",
	}, "\n"))
}

func TestPrintDiskUsageTableFormat(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	err := printDiskUsage(testDiskUsage(), types.SystemDfOptions{
		Stdout: stdout,
		// The \t is what a shell passes through literally, so it has to be expanded here.
		Format: `table {{.Type}}\t{{.Size}}`,
	})
	assert.NilError(t, err)

	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	assert.Equal(t, len(lines), 5)
	// The header is the same template over the column labels, so it names the chosen columns only.
	assert.Equal(t, strings.Join(strings.Fields(lines[0]), " "), "TYPE SIZE")
	assert.Equal(t, strings.Join(strings.Fields(lines[1]), " "), "Images 1kB")
	assert.Equal(t, strings.Join(strings.Fields(lines[4]), " "), "Build Cache 500B")
	// The columns are aligned, unlike a bare template.
	assert.Assert(t, strings.Contains(lines[1], "    "), lines[1])
}

func TestPrintDiskUsageTableFormatHeader(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	err := printDiskUsage(testDiskUsage(), types.SystemDfOptions{
		Stdout: stdout,
		Format: `table {{lower .Type}}\t{{truncate .Size 2}}`,
	})
	assert.NilError(t, err)

	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	// The header names the columns whatever the template does to the values under them, so the
	// functions that transform a value are not applied to the labels.
	assert.Equal(t, strings.Join(strings.Fields(lines[0]), " "), "TYPE SIZE")
	assert.Equal(t, strings.Join(strings.Fields(lines[1]), " "), "images 1k")
}

func TestPrintDiskUsageBareTableFormat(t *testing.T) {
	t.Parallel()

	// A bare "table" keeps the default columns.
	stdout := &bytes.Buffer{}
	err := printDiskUsage(testDiskUsage(), types.SystemDfOptions{Stdout: stdout, Format: "table"})
	assert.NilError(t, err)

	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	assert.Equal(t, strings.Join(strings.Fields(lines[0]), " "), "TYPE TOTAL ACTIVE SIZE RECLAIMABLE")
	assert.Equal(t, len(lines), 5)
}

func TestPrintDiskUsageFormatJSON(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	err := printDiskUsage(testDiskUsage(), types.SystemDfOptions{Stdout: stdout, Format: "json"})
	assert.NilError(t, err)

	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	assert.Equal(t, len(lines), 4)
	for _, line := range lines {
		var row dfPrintable
		assert.NilError(t, json.Unmarshal([]byte(line), &row))
		assert.Assert(t, row.Type != "", line)
	}
}

func TestPrintDiskUsageRawIsUnsupported(t *testing.T) {
	t.Parallel()

	err := printDiskUsage(testDiskUsage(), types.SystemDfOptions{Stdout: &bytes.Buffer{}, Format: "raw"})
	assert.ErrorContains(t, err, "raw")
}

func TestPrintDiskUsageVerbose(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	err := printDiskUsage(testDiskUsage(), types.SystemDfOptions{Stdout: stdout, Verbose: true})
	assert.NilError(t, err)

	out := stdout.String()
	for _, section := range []string{
		"Images space usage:",
		"Containers space usage:",
		"Local Volumes space usage:",
		"Build cache usage: 500B",
	} {
		assert.Assert(t, strings.Contains(out, section), out)
	}

	// UNIQUE SIZE is what is left once the shared part is taken out of the size.
	assert.Equal(t, fieldsOfRowContaining(out, "example.com/foo"),
		"example.com/foo latest 09538a1f51d3 About an hour ago 800B 200B 600B 1")
	// An image with neither repository nor tag is shown the Docker way.
	assert.Equal(t, fieldsOfRowContaining(out, "0168606be231"),
		"<none> <none> 0168606be231 About an hour ago 600B 200B 400B 0")
	// The build cache record keeps its opaque identifier, only truncated.
	assert.Equal(t, fieldsOfRowContaining(out, "n3vkjqf4tzxk"),
		"n3vkjqf4tzxk regular 500B About an hour ago About a minute ago 3 false")
}

func TestPrintDiskUsageVerboseMarksBuildCacheInUse(t *testing.T) {
	t.Parallel()

	du := testDiskUsage()
	du.BuildCache.Items[0].InUse = true

	stdout := &bytes.Buffer{}
	err := printDiskUsage(du, types.SystemDfOptions{Stdout: stdout, Verbose: true})
	assert.NilError(t, err)

	// Docker gives it no column of its own: a record in use is the one whose ID carries a star.
	assert.Equal(t, fieldsOfRowContaining(stdout.String(), "n3vkjqf4tzxk"),
		"n3vkjqf4tzxk* regular 500B About an hour ago About a minute ago 3 false")

	// A custom format can still ask for it by name.
	formatted := &bytes.Buffer{}
	err = printDiskUsage(du, types.SystemDfOptions{
		Stdout:  formatted,
		Verbose: true,
		Format:  `{{range .BuildCache}}{{.InUse}}{{end}}`,
	})
	assert.NilError(t, err)
	assert.Equal(t, strings.TrimSpace(formatted.String()), "true")
}

func TestPrintDiskUsageVerboseKeepsFullIDsForFormat(t *testing.T) {
	t.Parallel()

	du := testDiskUsage()

	table := &bytes.Buffer{}
	err := printDiskUsage(du, types.SystemDfOptions{Stdout: table, Verbose: true})
	assert.NilError(t, err)
	// The table is for reading, so the identifiers are shortened.
	assert.Assert(t, strings.Contains(table.String(), "09538a1f51d3"), table.String())
	assert.Assert(t, !strings.Contains(table.String(), du.Images.Items[0].ID), table.String())

	formatted := &bytes.Buffer{}
	err = printDiskUsage(du, types.SystemDfOptions{Stdout: formatted, Verbose: true, Format: "json"})
	assert.NilError(t, err)

	// A custom format is for machines, so the identifiers stay addressable.
	var verbose dfVerbosePrintable
	assert.NilError(t, json.Unmarshal([]byte(strings.TrimSpace(formatted.String())), &verbose))
	assert.Equal(t, verbose.Images[0].ID, du.Images.Items[0].ID)
	assert.Equal(t, verbose.Containers[0].ID, du.Containers.Items[0].ID)
	assert.Equal(t, verbose.BuildCache[0].ID, du.BuildCache.Items[0].ID)
}

func TestPrintDiskUsageVerboseTableFormatShortensIDs(t *testing.T) {
	t.Parallel()

	du := testDiskUsage()
	stdout := &bytes.Buffer{}
	err := printDiskUsage(du, types.SystemDfOptions{
		Stdout:  stdout,
		Verbose: true,
		Format:  `table {{range .Images}}{{.ID}}{{end}}`,
	})
	assert.NilError(t, err)

	// A table format is still a table, whatever columns it asks for, so Docker shortens its
	// identifiers just like those of the default one.
	assert.Assert(t, strings.Contains(stdout.String(), "09538a1f51d3"), stdout.String())
	assert.Assert(t, !strings.Contains(stdout.String(), du.Images.Items[0].ID), stdout.String())
}

func TestPrintDiskUsageVerboseFormat(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	err := printDiskUsage(testDiskUsage(), types.SystemDfOptions{
		Stdout:  stdout,
		Verbose: true,
		Format:  "json",
	})
	assert.NilError(t, err)

	var verbose dfVerbosePrintable
	assert.NilError(t, json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &verbose))
	assert.Equal(t, len(verbose.Images), 2)
	assert.Equal(t, len(verbose.Containers), 1)
	assert.Equal(t, len(verbose.Volumes), 2)
	assert.Equal(t, len(verbose.BuildCache), 1)
	assert.Equal(t, verbose.Images[0].UniqueSize, "600B")
}
