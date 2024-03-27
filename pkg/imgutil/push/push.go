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

// Package push derived from https://github.com/containerd/containerd/blob/v1.4.3/cmd/ctr/commands/images/push.go
package push

import (
	"context"
	"fmt"
	"io"
	"sync"
	"text/tabwriter"
	"time"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/remotes"
	"github.com/containerd/containerd/v2/core/remotes/docker"
	"github.com/containerd/containerd/v2/pkg/progress"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/imgutil/jobs"
	"github.com/containerd/platforms"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"golang.org/x/sync/errgroup"
)

// Push pushes an image to a remote registry.
func Push(ctx context.Context, client *containerd.Client, resolver remotes.Resolver, pushTracker docker.StatusTracker, stdout io.Writer,
	localRef, remoteRef string, platform platforms.MatchComparer, allowNonDist, quiet bool) error {
	img, err := client.ImageService().Get(ctx, localRef)
	if err != nil {
		return fmt.Errorf("unable to resolve image to manifest: %w", err)
	}
	desc := img.Target

	ongoing := newPushJobs(pushTracker)

	eg, ctx := errgroup.WithContext(ctx)

	// used to notify the progress writer
	doneCh := make(chan struct{})

	eg.Go(func() error {
		defer close(doneCh)

		log.G(ctx).WithField("image", remoteRef).WithField("digest", desc.Digest).Debug("pushing")

		jobHandler := images.HandlerFunc(func(ctx context.Context, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
			if allowNonDist || !images.IsNonDistributable(desc.MediaType) {
				ongoing.add(remotes.MakeRefKey(ctx, desc))
			}
			return nil, nil
		})

		if !allowNonDist {
			jobHandler = remotes.SkipNonDistributableBlobs(jobHandler)
		}

		return client.Push(ctx, remoteRef, desc,
			containerd.WithResolver(resolver),
			containerd.WithImageHandler(jobHandler),
			containerd.WithPlatformMatcher(platform),
		)
	})

	if !quiet {
		eg.Go(func() error {
			var (
				ticker = time.NewTicker(100 * time.Millisecond)
				fw     = progress.NewWriter(stdout)
				start  = time.Now()
				done   bool
			)

			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					fw.Flush()

					tw := tabwriter.NewWriter(fw, 1, 8, 1, ' ', 0)

					jobs.Display(tw, ongoing.status(), start)
					tw.Flush()

					if done {
						fw.Flush()
						return nil
					}
				case <-doneCh:
					done = true
				case <-ctx.Done():
					done = true // allow ui to update once more
				}
			}
		})
	}
	return eg.Wait()
}

type pushjobs struct {
	jobs    map[string]struct{}
	ordered []string
	tracker docker.StatusTracker
	mu      sync.Mutex
}

func newPushJobs(tracker docker.StatusTracker) *pushjobs {
	return &pushjobs{
		jobs:    make(map[string]struct{}),
		tracker: tracker,
	}
}

func (j *pushjobs) add(ref string) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if _, ok := j.jobs[ref]; ok {
		return
	}
	j.ordered = append(j.ordered, ref)
	j.jobs[ref] = struct{}{}
}

func (j *pushjobs) status() []jobs.StatusInfo {
	j.mu.Lock()
	defer j.mu.Unlock()

	statuses := make([]jobs.StatusInfo, 0, len(j.jobs))
	for _, name := range j.ordered {
		si := jobs.StatusInfo{
			Ref: name,
		}

		status, err := j.tracker.GetStatus(name)
		if err != nil {
			si.Status = "waiting"
		} else {
			si.Offset = status.Offset
			si.Total = status.Total
			si.StartedAt = status.StartedAt
			si.UpdatedAt = status.UpdatedAt
			if status.Offset >= status.Total {
				if status.UploadUUID == "" {
					si.Status = "done"
				} else {
					si.Status = "committing"
				}
			} else {
				si.Status = "uploading"
			}
		}
		statuses = append(statuses, si)
	}

	return statuses
}
