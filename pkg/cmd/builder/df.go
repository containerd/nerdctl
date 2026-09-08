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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/buildkitutil"
)

// DiskUsage reports how much disk space the BuildKit build cache uses.
//
// A record is active while a build holds it, and a record can be reclaimed when it is neither in use
// nor shared with another BuildKit worker, which is what `nerdctl builder prune` would free.
func DiskUsage(ctx context.Context, options types.BuilderDiskUsageOptions) (types.BuildCacheDiskUsage, error) {
	records, err := diskUsageRecords(ctx, options)
	if err != nil {
		return types.BuildCacheDiskUsage{}, err
	}
	return aggregateDiskUsage(records, options.Verbose), nil
}

// aggregateDiskUsage totals the build cache records the way Docker does.
func aggregateDiskUsage(records []buildkitutil.UsageInfo, verbose bool) types.BuildCacheDiskUsage {
	du := types.BuildCacheDiskUsage{}

	for _, record := range records {
		du.TotalCount++
		du.TotalSize += record.Size
		if record.InUse {
			du.ActiveCount++
		}
		if !record.InUse && !record.Shared {
			du.Reclaimable += record.Size
		}

		if verbose {
			du.Items = append(du.Items, types.BuildCacheDiskUsageItem{
				ID:         record.ID,
				CacheType:  string(record.RecordType),
				Size:       record.Size,
				CreatedAt:  record.CreatedAt,
				LastUsedAt: record.LastUsedAt,
				UsageCount: record.UsageCount,
				InUse:      record.InUse,
				Shared:     record.Shared,
			})
		}
	}

	return du
}

// diskUsageRecords runs `buildctl du` and decodes its output. Unlike `buildctl prune`, which streams
// one JSON object per pruned record, `buildctl du` applies the template to the whole result at once,
// so the output is a single JSON array.
func diskUsageRecords(ctx context.Context, options types.BuilderDiskUsageOptions) ([]buildkitutil.UsageInfo, error) {
	buildctlBinary, err := buildkitutil.BuildctlBinary()
	if err != nil {
		return nil, err
	}
	buildctlArgs := buildkitutil.BuildctlBaseArgs(options.BuildKitHost)
	buildctlArgs = append(buildctlArgs, "du", "--format={{json .}}")

	buildctlCmd := exec.CommandContext(ctx, buildctlBinary, buildctlArgs...)
	log.G(ctx).Debugf("running %v", buildctlCmd.Args)
	buildctlCmd.Stderr = options.Stderr
	stdout := &bytes.Buffer{}
	buildctlCmd.Stdout = stdout
	if err := buildctlCmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to run %v: %w", buildctlCmd.Args, err)
	}

	return parseDiskUsageRecords(stdout.Bytes())
}

// parseDiskUsageRecords decodes the JSON array `buildctl du --format={{json .}}` prints. An empty
// build cache is rendered as "null", which decodes into no records at all.
func parseDiskUsageRecords(output []byte) ([]buildkitutil.UsageInfo, error) {
	var records []buildkitutil.UsageInfo
	if err := json.Unmarshal(bytes.TrimSpace(output), &records); err != nil {
		return nil, fmt.Errorf("failed to decode the output of buildctl du: %w", err)
	}
	return records, nil
}
