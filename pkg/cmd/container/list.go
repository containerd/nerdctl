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
	"sync"
	"time"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/pkg/progress"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/containerdutil"
	"github.com/containerd/nerdctl/v2/pkg/containerutil"
	"github.com/containerd/nerdctl/v2/pkg/formatter"
	"github.com/containerd/nerdctl/v2/pkg/imgutil"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/portutil"
)

// List prints containers according to `options`.
func List(ctx context.Context, client *containerd.Client, options types.ContainerListOptions) ([]ListItem, error) {
	containers, cMap, err := filterContainers(ctx, client, options.Filters, options.LastN, options.All)
	if err != nil {
		return nil, err
	}
	return prepareContainers(ctx, client, &containers, cMap, options)
}

// filterContainers returns containers matching the filters.
//
//   - Supported filters: https://github.com/containerd/nerdctl/blob/main/docs/command-reference.md#whale-blue_square-nerdctl-ps
//   - all means showing all containers (default shows just running).
//   - lastN means only showing n last created containers (includes all states). Non-positive values are ignored.
//     In other words, if lastN is positive, all will be set to true.
func filterContainers(ctx context.Context, client *containerd.Client, filters []string, lastN int, all bool) ([]containerd.Container, map[string]string, error) {
	containers, err := client.Containers(ctx)
	if err != nil {
		return nil, nil, err
	}
	filterCtx, err := foldContainerFilters(ctx, containers, filters)
	if err != nil {
		return nil, nil, err
	}
	containers = filterCtx.MatchesFilters(ctx)

	sort.Slice(containers, func(i, j int) bool {
		infoI, _ := containers[i].Info(ctx, containerd.WithoutRefreshedMetadata)
		infoJ, _ := containers[j].Info(ctx, containerd.WithoutRefreshedMetadata)
		return infoI.CreatedAt.After(infoJ.CreatedAt)
	})

	if lastN > 0 {
		all = true
		if lastN < len(containers) {
			containers = containers[:lastN]
		}
	}

	var wg sync.WaitGroup
	statusPerContainer := make(map[string]string)
	var mu sync.Mutex
	// formatter.ContainerStatus(ctx, c) is time consuming so we do it in goroutines and return the container's id with status as a map.
	// prepareContainers func will use this map to avoid call formatter.ContainerStatus again.
	for _, c := range containers {
		if c.ID() == "" {
			return nil, nil, fmt.Errorf("container id is nill")
		}
		wg.Add(1)
		go func(ctx context.Context, c containerd.Container) {
			defer wg.Done()
			cStatus := formatter.ContainerStatus(ctx, c)
			mu.Lock()
			statusPerContainer[c.ID()] = cStatus
			mu.Unlock()
		}(ctx, c)
	}
	wg.Wait()
	if all || filterCtx.all {
		return containers, statusPerContainer, nil
	}

	var upContainers []containerd.Container
	for _, c := range containers {
		cStatus := statusPerContainer[c.ID()]
		if strings.HasPrefix(cStatus, "Up") {
			upContainers = append(upContainers, c)
		}
	}
	return upContainers, statusPerContainer, nil
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

func prepareContainers(ctx context.Context, client *containerd.Client, containers *[]containerd.Container, statusPerContainer map[string]string, options types.ContainerListOptions) ([]ListItem, error) {
	listItems := make([]ListItem, len(*containers))
	snapshottersCache := map[string]snapshots.Snapshotter{}
	for i := range *containers {
		info, err := (*containers)[i].Info(ctx, containerd.WithoutRefreshedMetadata)
		if err != nil {
			if errdefs.IsNotFound(err) {
				log.G(ctx).Warn(err)
				continue
			}
			return nil, err
		}
		id := (*containers)[i].ID()
		if options.Truncate && len(id) > 12 {
			id = id[:12]
		}
		var status string
		if s, ok := statusPerContainer[(*containers)[i].ID()]; ok {
			status = s
		} else {
			return nil, fmt.Errorf("can't get container %s status", (*containers)[i].ID())
		}
		dataStore, err := clientutil.DataStore(options.GOptions.DataRoot, options.GOptions.Address)
		if err != nil {
			return nil, err
		}
		containerLabels, err := (*containers)[i].Labels(ctx)
		if err != nil {
			return nil, err
		}
		ports, err := portutil.LoadPortMappings(dataStore, options.GOptions.Namespace, (*containers)[i].ID(), containerLabels)
		if err != nil {
			return nil, err
		}

		cmd, err := formatter.GetCommandFromSpec(info.Spec, options.Truncate, true)
		if err != nil {
			continue
		}
		li := ListItem{
			Command:   cmd,
			CreatedAt: info.CreatedAt,
			ID:        id,
			Image:     info.Image,
			Platform:  info.Labels[labels.Platform],
			Names:     containerutil.GetContainerName(info.Labels),
			Ports:     formatter.FormatPorts(ports),
			Status:    status,
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
		(*containers)[i] = nil
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
