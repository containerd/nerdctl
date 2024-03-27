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

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/pkg/progress"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/formatter"
	"github.com/containerd/nerdctl/v2/pkg/imgutil"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/labels/k8slabels"
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
	Labels    map[string]string
	// TODO: "LocalVolumes", "Mounts", "Networks", "RunningFor", "State"
}

func (x *ListItem) Label(s string) string {
	return x.Labels[s]
}

func prepareContainers(ctx context.Context, client *containerd.Client, containers []containerd.Container, options types.ContainerListOptions) ([]ListItem, error) {
	listItems := make([]ListItem, len(containers))
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
			Names:     getContainerName(info.Labels),
			Ports:     formatter.FormatPorts(info.Labels),
			Status:    formatter.ContainerStatus(ctx, c),
			Runtime:   info.Runtime.Name,
			Labels:    info.Labels,
		}
		if options.Size {
			containerSize, err := getContainerSize(ctx, client, c, info)
			if err != nil {
				return nil, err
			}
			li.Size = containerSize
		}
		listItems[i] = li
	}
	return listItems, nil
}

func getContainerName(containerLabels map[string]string) string {
	if name, ok := containerLabels[labels.Name]; ok {
		return name
	}

	if ns, ok := containerLabels[k8slabels.PodNamespace]; ok {
		if podName, ok := containerLabels[k8slabels.PodName]; ok {
			if containerName, ok := containerLabels[k8slabels.ContainerName]; ok {
				// Container
				return fmt.Sprintf("k8s://%s/%s/%s", ns, podName, containerName)
			}
			// Pod sandbox
			return fmt.Sprintf("k8s://%s/%s", ns, podName)
		}
	}
	return ""
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

func getContainerSize(ctx context.Context, client *containerd.Client, c containerd.Container, info containers.Container) (string, error) {
	// get container snapshot size
	snapshotKey := info.SnapshotKey
	var containerSize int64

	if snapshotKey != "" {
		usage, err := client.SnapshotService(info.Snapshotter).Usage(ctx, snapshotKey)
		if err != nil {
			return "", err
		}
		containerSize = usage.Size
	}

	// get the image interface
	image, err := c.Image(ctx)
	if err != nil {
		return "", err
	}

	sn := client.SnapshotService(info.Snapshotter)

	imageSize, err := imgutil.UnpackedImageSize(ctx, sn, image)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s (virtual %s)", progress.Bytes(containerSize).String(), progress.Bytes(imageSize).String()), nil
}
