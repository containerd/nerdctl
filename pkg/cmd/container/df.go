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

package container

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/containerdutil"
	"github.com/containerd/nerdctl/v2/pkg/containerutil"
	"github.com/containerd/nerdctl/v2/pkg/formatter"
	"github.com/containerd/nerdctl/v2/pkg/imgutil"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/mountutil"
)

// DiskUsage reports how much disk space the containers of the current namespace use.
//
// Like Docker, only the read-write layer is counted: the layers coming from the image belong to the
// image, not to the container. Every container is counted, including the stopped ones, and the space
// of everything that is not running can be reclaimed.
func DiskUsage(ctx context.Context, client *containerd.Client, _ types.GlobalCommandOptions, verbose bool) (types.ContainerDiskUsage, error) {
	du := types.ContainerDiskUsage{}

	containers, err := client.Containers(ctx)
	if err != nil {
		return du, err
	}

	snapshottersCache := map[string]snapshots.Snapshotter{}
	for _, c := range containers {
		info, err := c.Info(ctx, containerd.WithoutRefreshedMetadata)
		if err != nil {
			// There is no guarantee that a container we just listed still exists.
			if errdefs.IsNotFound(err) {
				log.G(ctx).Debugf("container %q is gone - ignoring", c.ID())
				continue
			}
			return du, err
		}

		snapshotter, ok := snapshottersCache[info.Snapshotter]
		if !ok {
			snapshotter = containerdutil.SnapshotService(client, info.Snapshotter)
			snapshottersCache[info.Snapshotter] = snapshotter
		}

		var sizeRw int64
		if info.SnapshotKey != "" {
			rw, _, err := imgutil.ResourceUsage(ctx, snapshotter, info.SnapshotKey)
			if err != nil {
				// ResourceUsage walks the whole snapshot chain, so a NotFound may mean the container
				// was removed while we were measuring it, but it may just as well mean a missing
				// parent snapshot of a container that is still there. Only the container store can
				// tell the two apart, and dropping a live container would understate the report as
				// silently as counting it as empty would.
				if errdefs.IsNotFound(err) && containerIsGone(ctx, client, c.ID()) {
					log.G(ctx).Debugf("container %q is gone - ignoring", c.ID())
					continue
				}
				return du, fmt.Errorf("failed to get the size of container %q: %w", c.ID(), err)
			}
			sizeRw = rw.Size
		}

		status := formatter.ContainerStatus(ctx, c)

		du.TotalCount++
		du.TotalSize += sizeRw
		if isActiveStatus(status) {
			du.ActiveCount++
		} else {
			du.Reclaimable += sizeRw
		}

		if verbose {
			item := types.ContainerDiskUsageItem{
				ID:           c.ID(),
				Image:        info.Image,
				LocalVolumes: localVolumes(ctx, info.Labels),
				SizeRw:       sizeRw,
				CreatedAt:    info.CreatedAt,
				Status:       status,
				Names:        containerutil.GetContainerName(info.Labels),
			}
			if spec, err := c.Spec(ctx); err != nil {
				log.G(ctx).WithError(err).Debugf("failed to get the spec of container %q", c.ID())
			} else {
				item.Command = formatter.InspectContainerCommand(spec, true, true)
			}
			du.Items = append(du.Items, item)
		}
	}

	return du, nil
}

// containerIsGone reports whether a container no longer exists, asking the container store rather
// than any metadata that was read before.
func containerIsGone(ctx context.Context, client *containerd.Client, id string) bool {
	_, err := client.ContainerService().Get(ctx, id)
	return errdefs.IsNotFound(err)
}

// isActiveStatus reports whether a container occupies space that cannot be reclaimed. Docker treats
// the running, paused and restarting containers as active; the status strings are the ones produced
// by formatter.ContainerStatus.
func isActiveStatus(status string) bool {
	for _, prefix := range []string{"Up", "Paused", "Pausing", "Restarting"} {
		if strings.HasPrefix(status, prefix) {
			return true
		}
	}
	return false
}

// localVolumes returns the number of named and anonymous volumes a container mounts.
func localVolumes(ctx context.Context, containerLabels map[string]string) int64 {
	mountsJSON := labels.GetMount(containerLabels)
	if mountsJSON == "" {
		return 0
	}
	var mounts []dockercompat.MountPoint
	if err := json.Unmarshal([]byte(mountsJSON), &mounts); err != nil {
		log.G(ctx).WithError(err).Debug("failed to parse the mounts of a container")
		return 0
	}
	var count int64
	for _, m := range mounts {
		if m.Type == mountutil.Volume {
			count++
		}
	}
	return count
}
