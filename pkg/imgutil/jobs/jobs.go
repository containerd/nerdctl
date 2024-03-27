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

package jobs

import (
	"context"
	"fmt"
	"io"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/remotes"
	"github.com/containerd/containerd/v2/pkg/progress"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// ShowProgress continuously updates the output with job progress
// by checking status in the content store.
//
// From https://github.com/containerd/containerd/blob/v1.7.0-rc.2/cmd/ctr/commands/content/fetch.go#L219-L336
func ShowProgress(ctx context.Context, ongoing *Jobs, cs content.Store, out io.Writer) {
	var (
		ticker   = time.NewTicker(100 * time.Millisecond)
		fw       = progress.NewWriter(out)
		start    = time.Now()
		statuses = map[string]StatusInfo{}
		done     bool
	)
	defer ticker.Stop()

outer:
	for {
		select {
		case <-ticker.C:
			fw.Flush()

			tw := tabwriter.NewWriter(fw, 1, 8, 1, ' ', 0)

			resolved := StatusResolved
			if !ongoing.IsResolved() {
				resolved = StatusResolving
			}
			statuses[ongoing.name] = StatusInfo{
				Ref:    ongoing.name,
				Status: resolved,
			}
			keys := []string{ongoing.name}

			activeSeen := map[string]struct{}{}
			if !done {
				active, err := cs.ListStatuses(ctx, "")
				if err != nil {
					log.G(ctx).WithError(err).Error("active check failed")
					continue
				}
				// update status of active entries!
				for _, active := range active {
					statuses[active.Ref] = StatusInfo{
						Ref:       active.Ref,
						Status:    StatusDownloading,
						Offset:    active.Offset,
						Total:     active.Total,
						StartedAt: active.StartedAt,
						UpdatedAt: active.UpdatedAt,
					}
					activeSeen[active.Ref] = struct{}{}
				}
			}

			// now, update the items in jobs that are not in active
			for _, j := range ongoing.Jobs() {
				key := remotes.MakeRefKey(ctx, j)
				keys = append(keys, key)
				if _, ok := activeSeen[key]; ok {
					continue
				}

				status, ok := statuses[key]
				if !done && (!ok || status.Status == StatusDownloading) {
					info, err := cs.Info(ctx, j.Digest)
					if err != nil {
						if !errdefs.IsNotFound(err) {
							log.G(ctx).WithError(err).Error("failed to get content info")
							continue outer
						}
						statuses[key] = StatusInfo{
							Ref:    key,
							Status: StatusWaiting,
						}
					} else if info.CreatedAt.After(start) {
						statuses[key] = StatusInfo{
							Ref:       key,
							Status:    StatusDone,
							Offset:    info.Size,
							Total:     info.Size,
							UpdatedAt: info.CreatedAt,
						}
					} else {
						statuses[key] = StatusInfo{
							Ref:    key,
							Status: StatusExists,
						}
					}
				} else if done {
					if ok {
						if status.Status != StatusDone && status.Status != StatusExists {
							status.Status = StatusDone
							statuses[key] = status
						}
					} else {
						statuses[key] = StatusInfo{
							Ref:    key,
							Status: StatusDone,
						}
					}
				}
			}

			var ordered []StatusInfo
			for _, key := range keys {
				ordered = append(ordered, statuses[key])
			}

			Display(tw, ordered, start)
			tw.Flush()

			if done {
				fw.Flush()
				return
			}
		case <-ctx.Done():
			done = true // allow ui to update once more
		}
	}
}

// Jobs provides a way of identifying the download keys for a particular task
// encountering during the pull walk.
//
// This is very minimal and will probably be replaced with something more
// featured.
//
// From https://github.com/containerd/containerd/blob/v1.7.0-rc.2/cmd/ctr/commands/content/fetch.go#L338-L349
type Jobs struct {
	name     string
	added    map[digest.Digest]struct{}
	descs    []ocispec.Descriptor
	mu       sync.Mutex
	resolved bool
}

// New creates a new instance of the job status tracker.
// From https://github.com/containerd/containerd/blob/v1.7.0-rc.2/cmd/ctr/commands/content/fetch.go#L351-L357
func New(name string) *Jobs {
	return &Jobs{
		name:  name,
		added: map[digest.Digest]struct{}{},
	}
}

// Add adds a descriptor to be tracked.
// From https://github.com/containerd/containerd/blob/v1.7.0-rc.2/cmd/ctr/commands/content/fetch.go#L359-L370
func (j *Jobs) Add(desc ocispec.Descriptor) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.resolved = true

	if _, ok := j.added[desc.Digest]; ok {
		return
	}
	j.descs = append(j.descs, desc)
	j.added[desc.Digest] = struct{}{}
}

// Jobs returns a list of all tracked descriptors.
// From https://github.com/containerd/containerd/blob/v1.7.0-rc.2/cmd/ctr/commands/content/fetch.go#L372-L379
func (j *Jobs) Jobs() []ocispec.Descriptor {
	j.mu.Lock()
	defer j.mu.Unlock()

	var descs []ocispec.Descriptor
	return append(descs, j.descs...)
}

// IsResolved checks whether a descriptor has been resolved.
// From https://github.com/containerd/containerd/blob/v1.7.0-rc.2/cmd/ctr/commands/content/fetch.go#L381-L386
func (j *Jobs) IsResolved() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.resolved
}

// StatusInfoStatus describes status info for an upload or download.
// From https://github.com/containerd/containerd/blob/v1.7.0-rc.2/cmd/ctr/commands/content/fetch.go#L388-L400
type StatusInfoStatus string

const (
	StatusResolved    StatusInfoStatus = "resolved"
	StatusResolving   StatusInfoStatus = "resolving"
	StatusWaiting     StatusInfoStatus = "waiting"
	StatusCommitting  StatusInfoStatus = "committing"
	StatusDone        StatusInfoStatus = "done"
	StatusDownloading StatusInfoStatus = "downloading"
	StatusUploading   StatusInfoStatus = "uploading"
	StatusExists      StatusInfoStatus = "exists"
)

// StatusInfo holds the status info for an upload or download.
// From https://github.com/containerd/containerd/blob/v1.7.0-rc.2/cmd/ctr/commands/content/fetch.go#L402-L410
type StatusInfo struct {
	Ref       string
	Status    StatusInfoStatus
	Offset    int64
	Total     int64
	StartedAt time.Time
	UpdatedAt time.Time
}

// Display pretty prints out the download or upload progress.
// From https://github.com/containerd/containerd/blob/v1.7.0-rc.2/cmd/ctr/commands/content/fetch.go#L412-L452
func Display(w io.Writer, statuses []StatusInfo, start time.Time) {
	var total int64
	for _, status := range statuses {
		total += status.Offset
		switch status.Status {
		case StatusDownloading, StatusUploading:
			var bar progress.Bar
			if status.Total > 0.0 {
				bar = progress.Bar(float64(status.Offset) / float64(status.Total))
			}
			fmt.Fprintf(w, "%s:\t%s\t%40r\t%8.8s/%s\t\n",
				status.Ref,
				status.Status,
				bar,
				progress.Bytes(status.Offset), progress.Bytes(status.Total))
		case StatusResolving, StatusWaiting:
			bar := progress.Bar(0.0)
			fmt.Fprintf(w, "%s:\t%s\t%40r\t\n",
				status.Ref,
				status.Status,
				bar)
		default:
			bar := progress.Bar(1.0)
			fmt.Fprintf(w, "%s:\t%s\t%40r\t\n",
				status.Ref,
				status.Status,
				bar)
		}
	}

	fmt.Fprintf(w, "elapsed: %-4.1fs\ttotal: %7.6v\t(%v)\t\n",
		time.Since(start).Seconds(),
		// TODO(stevvooe): These calculations are actually way off.
		// Need to account for previously downloaded data. These
		// will basically be right for a download the first time
		// but will be skewed if restarting, as it includes the
		// data into the start time before.
		progress.Bytes(total),
		progress.NewBytesPerSecond(total, time.Since(start)))
}
