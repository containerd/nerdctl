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
	"sort"
	"strings"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/pkg/progress"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/containerdutil"
	"github.com/containerd/nerdctl/v2/pkg/containerutil"
	"github.com/containerd/nerdctl/v2/pkg/formatter"
	"github.com/containerd/nerdctl/v2/pkg/imgutil"
	"github.com/containerd/nerdctl/v2/pkg/labels"
)

// List prints containers according to `options`.
func List(ctx context.Context, client *containerd.Client, options types.ContainerListOptions) ([]ListItem, error) {
	containers, err := filterContainers(ctx, client, options.Filters, options.LastN, options.All)
	if err != nil {
		return nil, err
	}
	return prepareContainers(ctx, client, containers, options)
}

// filterContainers returns containers matching the filters.
//
//   - Supported filters: https://github.com/containerd/nerdctl/blob/main/docs/command-reference.md#whale-blue_square-nerdctl-ps
//   - all means showing all containers (default shows just running).
//   - lastN means only showing n last created containers (includes all states). Non-positive values are ignored.
//     In other words, if lastN is positive, all will be set to true.
func filterContainers(ctx context.Context, client *containerd.Client, filters []string, lastN int, all bool) ([]containerd.Container, error) {
	containers, err := client.Containers(ctx)
	if err != nil {
		return nil, err
	}
	filterCtx, err := foldContainerFilters(ctx, containers, filters)
	if err != nil {
		return nil, err
	}
	containers = filterCtx.MatchesFilters(ctx)
	if lastN > 0 {
		all = true
		sort.Slice(containers, func(i, j int) bool {
			infoI, _ := containers[i].Info(ctx, containerd.WithoutRefreshedMetadata)
			infoJ, _ := containers[j].Info(ctx, containerd.WithoutRefreshedMetadata)
			return infoI.CreatedAt.After(infoJ.CreatedAt)
		})
		if lastN < len(containers) {
			containers = containers[:lastN]
		}
	}

	if all {
		return containers, nil
	}
	var upContainers []containerd.Container
	for _, c := range containers {
		cStatus := formatter.ContainerStatus(ctx, c)
		if strings.HasPrefix(cStatus, "Up") {
			upContainers = append(upContainers, c)
		}
	}
	return upContainers, nil
}

type ListItem struct {
	Command   string
	CreatedAt time.Time
	ID        string
	Image     string
	Platform  string // nerdctl extension
	Names     string
	Ports     string
	Status    string
	Runtime   string // nerdctl extension
	Size      string
	Labels    string
	LabelsMap map[string]string `json:"-"`

	// TODO: "LocalVolumes", "Mounts", "Networks", "RunningFor", "State"
}

func (x *ListItem) Label(s string) string {
	return x.LabelsMap[s]
}

func prepareContainers(ctx context.Context, client *containerd.Client, containers []containerd.Container, options types.ContainerListOptions) ([]ListItem, error) {
	listItems := make([]ListItem, len(containers))
	snapshottersCache := map[string]snapshots.Snapshotter{}
	for i, c := range containers {
		info, err := c.Info(ctx, containerd.WithoutRefreshedMetadata)
		if err != nil {
			if errdefs.IsNotFound(err) {
				log.G(ctx).Warn(err)
				continue
			}
			return nil, err
		}
		spec, err := c.Spec(ctx)
		if err != nil {
			if errdefs.IsNotFound(err) {
				log.G(ctx).Warn(err)
				continue
			}
			return nil, err
		}
		id := c.ID()
		if options.Truncate && len(id) > 12 {
			id = id[:12]
		}
		li := ListItem{
			Command:   formatter.InspectContainerCommand(spec, options.Truncate, true),
			CreatedAt: info.CreatedAt,
			ID:        id,
			Image:     info.Image,
			Platform:  info.Labels[labels.Platform],
			Names:     containerutil.GetContainerName(info.Labels),
			Ports:     formatter.FormatPorts(info.Labels),
			Status:    formatter.ContainerStatus(ctx, c),
			Runtime:   info.Runtime.Name,
			Labels:    formatter.FormatLabels(info.Labels),
			LabelsMap: info.Labels,
		}
		if options.Size {
			snapshotter, ok := snapshottersCache[info.Snapshotter]
			if !ok {
				snapshottersCache[info.Snapshotter] = containerdutil.SnapshotService(client, info.Snapshotter)
				snapshotter = snapshottersCache[info.Snapshotter]
			}
			containerSize, err := getContainerSize(ctx, snapshotter, info.SnapshotKey)
			if err != nil {
				return nil, err
			}
			li.Size = containerSize
		}
		listItems[i] = li
	}
	return listItems, nil
}

func getContainerNetworks(containerLables map[string]string) []string {
	var networks []string
	if names, ok := containerLables[labels.Networks]; ok {
		if err := json.Unmarshal([]byte(names), &networks); err != nil {
			log.L.Warn(err)
		}
	}
	return networks
}

func getContainerSize(ctx context.Context, snapshotter snapshots.Snapshotter, snapshotKey string) (string, error) {
	// get container snapshot size
	var containerSize int64
	var imageSize int64

	if snapshotKey != "" {
		rw, all, err := imgutil.ResourceUsage(ctx, snapshotter, snapshotKey)
		if err != nil {
			return "", err
		}
		containerSize = rw.Size
		imageSize = all.Size
	}

	return fmt.Sprintf("%s (virtual %s)", progress.Bytes(containerSize).String(), progress.Bytes(imageSize).String()), nil
}
