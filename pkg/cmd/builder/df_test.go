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

package builder

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/buildkitutil"
)

// buildctl du renders the whole result with one template pass, so the output is a JSON array, not
// the stream of objects buildctl prune produces.
const buildctlDuOutput = `[{"id":"n3vkjqf4tzxkgxwjdgm0e5vpm","mutable":false,"inUse":true,"size":102400,` +
	`"createdAt":"2026-08-01T10:00:00Z","lastUsedAt":"2026-08-04T10:00:00Z","usageCount":2,` +
	`"description":"pulled from docker.io/library/alpine:latest","recordType":"regular","shared":false},` +
	`{"id":"xk3f4tzqjn0e5vpmgxwjdgm0e","mutable":false,"inUse":false,"size":2048,` +
	`"createdAt":"2026-08-02T10:00:00Z","lastUsedAt":null,"usageCount":0,` +
	`"description":"local source for context","recordType":"source.local","shared":false},` +
	`{"id":"gm0e5vpmxk3f4tzqjn0egxwj","mutable":false,"inUse":false,"size":4096,` +
	`"createdAt":"2026-08-03T10:00:00Z","lastUsedAt":null,"usageCount":0,` +
	`"description":"shared with another worker","recordType":"regular","shared":true}]
`

func TestParseDiskUsageRecords(t *testing.T) {
	t.Parallel()

	records, err := parseDiskUsageRecords([]byte(buildctlDuOutput))
	assert.NilError(t, err)
	assert.Equal(t, len(records), 3)
	assert.Equal(t, records[0].ID, "n3vkjqf4tzxkgxwjdgm0e5vpm")
	assert.Equal(t, records[0].InUse, true)
	assert.Equal(t, records[0].UsageCount, 2)
	assert.Assert(t, records[0].LastUsedAt != nil)
	assert.Equal(t, string(records[1].RecordType), "source.local")
	assert.Assert(t, records[1].LastUsedAt == nil)
	assert.Equal(t, records[2].Shared, true)
}

func TestParseDiskUsageRecordsEmpty(t *testing.T) {
	t.Parallel()

	// An empty build cache is rendered by the Go template as "null".
	records, err := parseDiskUsageRecords([]byte("null\n"))
	assert.NilError(t, err)
	assert.Equal(t, len(records), 0)
}

func TestParseDiskUsageRecordsInvalid(t *testing.T) {
	t.Parallel()

	_, err := parseDiskUsageRecords([]byte("not json"))
	assert.ErrorContains(t, err, "buildctl du")
}

func TestBuildCacheDiskUsageAggregation(t *testing.T) {
	t.Parallel()

	records, err := parseDiskUsageRecords([]byte(buildctlDuOutput))
	assert.NilError(t, err)

	du := aggregateDiskUsage(records, true)
	assert.Equal(t, du.TotalCount, int64(3))
	assert.Equal(t, du.TotalSize, int64(102400+2048+4096))
	// Only the record a build holds is active.
	assert.Equal(t, du.ActiveCount, int64(1))
	// Neither the in-use record nor the one shared with another worker can be reclaimed.
	assert.Equal(t, du.Reclaimable, int64(2048))
	assert.Equal(t, len(du.Items), 3)
	assert.Equal(t, du.Items[0].CacheType, "regular")
}

func TestBuildCacheDiskUsageWithoutVerbose(t *testing.T) {
	t.Parallel()

	records, err := parseDiskUsageRecords([]byte(buildctlDuOutput))
	assert.NilError(t, err)

	du := aggregateDiskUsage(records, false)
	assert.Equal(t, du.TotalCount, int64(3))
	// The individual records are only carried when they are going to be printed.
	assert.Equal(t, len(du.Items), 0)
}

func TestBuildCacheDiskUsageNoRecords(t *testing.T) {
	t.Parallel()

	du := aggregateDiskUsage([]buildkitutil.UsageInfo{}, true)
	assert.DeepEqual(t, du, types.BuildCacheDiskUsage{})
}
