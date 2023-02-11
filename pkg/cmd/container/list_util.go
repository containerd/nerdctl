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
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/sirupsen/logrus"
)

func foldContainerFilters(ctx context.Context, containers []containerd.Container, filters []string) (*containerFilterContext, error) {
	filterCtx := &containerFilterContext{containers: containers}
	err := filterCtx.foldFilters(ctx, filters)
	return filterCtx, err
}

type containerFilterContext struct {
	containers []containerd.Container

	idFilterFuncs      []func(string) bool
	nameFilterFuncs    []func(string) bool
	exitedFilterFuncs  []func(int) bool
	beforeFilterFuncs  []func(t time.Time) bool
	sinceFilterFuncs   []func(t time.Time) bool
	statusFilterFuncs  []func(containerd.ProcessStatus) bool
	labelFilterFuncs   []func(map[string]string) bool
	volumeFilterFuncs  []func([]*containerVolume) bool
	networkFilterFuncs []func([]string) bool
}

func (cl *containerFilterContext) MatchesFilters(ctx context.Context) []containerd.Container {
	matchesContainers := make([]containerd.Container, 0, len(cl.containers))
	for _, container := range cl.containers {
		if !cl.matchesInfoFilters(ctx, container) {
			continue
		}
		if !cl.matchesTaskFilters(ctx, container) {
			continue
		}
		matchesContainers = append(matchesContainers, container)
	}
	cl.containers = matchesContainers
	return cl.containers
}

func (cl *containerFilterContext) foldFilters(ctx context.Context, filters []string) error {
	folders := []struct {
		filterType string
		foldFunc   func(context.Context, string, string) error
	}{
		{"id", cl.foldIDFilter}, {"name", cl.foldNameFilter},
		{"before", cl.foldBeforeFilter}, {"since", cl.foldSinceFilter},
		{"network", cl.foldNetworkFilter}, {"label", cl.foldLabelFilter},
		{"volume", cl.foldVolumeFilter}, {"status", cl.foldStatusFilter},
		{"exited", cl.foldExitedFilter},
	}
	for _, filter := range filters {
		invalidFilter := true
		for _, folder := range folders {
			if !strings.HasPrefix(filter, folder.filterType) {
				continue
			}
			splited := strings.SplitN(filter, "=", 2)
			if len(splited) != 2 {
				return fmt.Errorf("invalid argument \"%s\" for \"-f, --filter\": bad format of filter (expected name=value)", folder.filterType)
			}
			if err := folder.foldFunc(ctx, filter, splited[1]); err != nil {
				return err
			}
			invalidFilter = false
			break
		}
		if invalidFilter {
			return fmt.Errorf("invalid filter '%s'", filter)
		}
	}
	return nil
}

func (cl *containerFilterContext) foldExitedFilter(_ context.Context, filter, value string) error {
	exited, err := strconv.Atoi(value)
	if err != nil {
		return err
	}
	cl.exitedFilterFuncs = append(cl.exitedFilterFuncs, func(exitStatus int) bool {
		return exited == exitStatus
	})
	return nil
}

func (cl *containerFilterContext) foldStatusFilter(_ context.Context, filter, value string) error {
	status := containerd.ProcessStatus(value)
	switch status {
	case containerd.Running, containerd.Created, containerd.Stopped, containerd.Paused, containerd.Pausing, containerd.Unknown:
		cl.statusFilterFuncs = append(cl.statusFilterFuncs, func(stats containerd.ProcessStatus) bool {
			return status == stats
		})
	case containerd.ProcessStatus("exited"):
		cl.statusFilterFuncs = append(cl.statusFilterFuncs, func(stats containerd.ProcessStatus) bool {
			return containerd.Stopped == stats
		})
	case containerd.ProcessStatus("restarting"), containerd.ProcessStatus("removing"), containerd.ProcessStatus("dead"):
		logrus.Warnf("%s is not supported and is ignored", filter)
	default:
		return fmt.Errorf("invalid filter '%s'", filter)
	}
	return nil
}

func (cl *containerFilterContext) foldBeforeFilter(ctx context.Context, filter, value string) error {
	beforeC, err := idOrNameFilter(ctx, cl.containers, value)
	if err == nil {
		cl.beforeFilterFuncs = append(cl.beforeFilterFuncs, func(t time.Time) bool {
			return t.Before(beforeC.CreatedAt)
		})
	}
	return err
}

func (cl *containerFilterContext) foldSinceFilter(ctx context.Context, filter, value string) error {
	sinceC, err := idOrNameFilter(ctx, cl.containers, value)
	if err == nil {
		cl.sinceFilterFuncs = append(cl.sinceFilterFuncs, func(t time.Time) bool {
			return t.After(sinceC.CreatedAt)
		})
	}
	return err
}

func (cl *containerFilterContext) foldIDFilter(_ context.Context, filter, value string) error {
	cl.idFilterFuncs = append(cl.idFilterFuncs, func(id string) bool {
		if value == "" {
			return false
		}
		return strings.HasPrefix(id, value)
	})
	return nil
}

func (cl *containerFilterContext) foldNameFilter(_ context.Context, filter, value string) error {
	cl.nameFilterFuncs = append(cl.nameFilterFuncs, func(name string) bool {
		if value == "" {
			return true
		}
		return strings.Contains(name, value)
	})
	return nil
}

func (cl *containerFilterContext) foldLabelFilter(_ context.Context, filter, value string) error {
	k, v, hasValue := value, "", false
	if subs := strings.SplitN(value, "=", 2); len(subs) == 2 {
		hasValue = true
		k, v = subs[0], subs[1]
	}
	cl.labelFilterFuncs = append(cl.labelFilterFuncs, func(labels map[string]string) bool {
		if labels == nil {
			return false
		}
		val, ok := labels[k]
		if !ok || (hasValue && val != v) {
			return false
		}
		return true
	})
	return nil
}

func (cl *containerFilterContext) foldVolumeFilter(_ context.Context, filter, value string) error {
	cl.volumeFilterFuncs = append(cl.volumeFilterFuncs, func(vols []*containerVolume) bool {
		for _, vol := range vols {
			if (vol.Source != "" && vol.Source == value) ||
				(vol.Destination != "" && vol.Destination == value) ||
				(vol.Name != "" && vol.Name == value) {
				return true
			}
		}
		return false
	})
	return nil
}

func (cl *containerFilterContext) foldNetworkFilter(_ context.Context, filter, value string) error {
	cl.networkFilterFuncs = append(cl.networkFilterFuncs, func(networks []string) bool {
		for _, network := range networks {
			if network == value {
				return true
			}
		}
		return false
	})
	return nil
}

func (cl *containerFilterContext) matchesInfoFilters(ctx context.Context, container containerd.Container) bool {
	if len(cl.idFilterFuncs)+len(cl.nameFilterFuncs)+len(cl.beforeFilterFuncs)+
		len(cl.sinceFilterFuncs)+len(cl.labelFilterFuncs)+len(cl.volumeFilterFuncs)+len(cl.networkFilterFuncs) == 0 {
		return true
	}
	info, _ := container.Info(ctx, containerd.WithoutRefreshedMetadata)
	return cl.matchesIDFilter(info) && cl.matchesNameFilter(info) && cl.matchesBeforeFilter(info) &&
		cl.matchesSinceFilter(info) && cl.matchesLabelFilter(info) && cl.matchesVolumeFilter(info) &&
		cl.matchesNetworkFilter(info)
}

func (cl *containerFilterContext) matchesTaskFilters(ctx context.Context, container containerd.Container) bool {
	if len(cl.exitedFilterFuncs)+len(cl.statusFilterFuncs) == 0 {
		return true
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	task, err := container.Task(ctx, nil)
	if err != nil {
		logrus.Warn(err)
		return false
	}
	status, err := task.Status(ctx)
	if err != nil {
		logrus.Warn(err)
		return false
	}
	return cl.matchesExitedFilter(status) && cl.matchesStatusFilter(status)
}

func (cl *containerFilterContext) matchesExitedFilter(status containerd.Status) bool {
	if len(cl.exitedFilterFuncs) == 0 {
		return true
	}
	if status.Status != containerd.Stopped {
		return false
	}
	for _, exitedFilterFunc := range cl.exitedFilterFuncs {
		if !exitedFilterFunc(int(status.ExitStatus)) {
			continue
		}
		return true
	}
	return false
}

func (cl *containerFilterContext) matchesStatusFilter(status containerd.Status) bool {
	if len(cl.statusFilterFuncs) == 0 {
		return true
	}
	for _, statusFilterFunc := range cl.statusFilterFuncs {
		if !statusFilterFunc(status.Status) {
			continue
		}
		return true
	}
	return false
}

func (cl *containerFilterContext) matchesIDFilter(info containers.Container) bool {
	if len(cl.idFilterFuncs) == 0 {
		return true
	}
	for _, idFilterFunc := range cl.idFilterFuncs {
		if !idFilterFunc(info.ID) {
			continue
		}
		return true
	}
	return false
}

func (cl *containerFilterContext) matchesNameFilter(info containers.Container) bool {
	if len(cl.nameFilterFuncs) == 0 {
		return true
	}
	cName := getPrintableContainerName(info.Labels)
	for _, nameFilterFunc := range cl.nameFilterFuncs {
		if !nameFilterFunc(cName) {
			continue
		}
		return true
	}
	return false
}

func (cl *containerFilterContext) matchesSinceFilter(info containers.Container) bool {
	if len(cl.sinceFilterFuncs) == 0 {
		return true
	}
	for _, sinceFilterFunc := range cl.sinceFilterFuncs {
		if !sinceFilterFunc(info.CreatedAt) {
			continue
		}
		return true
	}
	return false
}

func (cl *containerFilterContext) matchesBeforeFilter(info containers.Container) bool {
	if len(cl.beforeFilterFuncs) == 0 {
		return true
	}
	for _, beforeFilterFunc := range cl.beforeFilterFuncs {
		if !beforeFilterFunc(info.CreatedAt) {
			continue
		}
		return true
	}
	return false
}

func (cl *containerFilterContext) matchesLabelFilter(info containers.Container) bool {
	for _, labelFilterFunc := range cl.labelFilterFuncs {
		if !labelFilterFunc(info.Labels) {
			return false
		}
	}
	return true
}

func (cl *containerFilterContext) matchesVolumeFilter(info containers.Container) bool {
	if len(cl.volumeFilterFuncs) == 0 {
		return true
	}
	vols := getContainerVolumes(info.Labels)
	for _, volumeFilterFunc := range cl.volumeFilterFuncs {
		if !volumeFilterFunc(vols) {
			continue
		}
		return true
	}
	return false
}

func (cl *containerFilterContext) matchesNetworkFilter(info containers.Container) bool {
	if len(cl.networkFilterFuncs) == 0 {
		return true
	}
	networks := getContainerNetworks(info.Labels)
	for _, networkFilterFunc := range cl.networkFilterFuncs {
		if !networkFilterFunc(networks) {
			continue
		}
		return true
	}
	return false
}

func idOrNameFilter(ctx context.Context, containers []containerd.Container, value string) (*containers.Container, error) {
	for _, container := range containers {
		info, err := container.Info(ctx, containerd.WithoutRefreshedMetadata)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(info.ID, value) || strings.Contains(getPrintableContainerName(info.Labels), value) {
			return &info, nil
		}
	}
	return nil, fmt.Errorf("no such container %s", value)
}
