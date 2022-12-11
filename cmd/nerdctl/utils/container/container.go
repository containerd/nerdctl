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
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/pkg/progress"
	"github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/common"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/volume"
	"github.com/containerd/nerdctl/pkg/dnsutil/hostsstore"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/namestore"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func FoldContainerFilters(ctx context.Context, containers []containerd.Container, filters []string) (*FilterContext, error) {
	filterCtx := &FilterContext{Containers: containers}
	err := filterCtx.foldFilters(ctx, filters)
	return filterCtx, err
}

type FilterContext struct {
	Containers []containerd.Container

	IDFilterFuncs      []func(string) bool
	NameFilterFuncs    []func(string) bool
	ExitedFilterFuncs  []func(int) bool
	BeforeFilterFuncs  []func(t time.Time) bool
	SinceFilterFuncs   []func(t time.Time) bool
	StatusFilterFuncs  []func(containerd.ProcessStatus) bool
	LabelFilterFuncs   []func(map[string]string) bool
	VolumeFilterFuncs  []func([]*volume.ContainerVolume) bool
	NetworkFilterFuncs []func([]string) bool
}

func (cl *FilterContext) MatchesFilters(ctx context.Context) []containerd.Container {
	matchesContainers := make([]containerd.Container, 0, len(cl.Containers))
	for _, container := range cl.Containers {
		if !cl.matchesInfoFilters(ctx, container) {
			continue
		}
		if !cl.matchesTaskFilters(ctx, container) {
			continue
		}
		matchesContainers = append(matchesContainers, container)
	}
	cl.Containers = matchesContainers
	return cl.Containers
}

func (cl *FilterContext) foldFilters(ctx context.Context, filters []string) error {
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
				return fmt.Errorf("invalid argument \"%s\" for \"-f, --filter\": bad format of filter (expected Name=value)", folder.filterType)
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

func (cl *FilterContext) foldExitedFilter(_ context.Context, filter, value string) error {
	exited, err := strconv.Atoi(value)
	if err != nil {
		return err
	}
	cl.ExitedFilterFuncs = append(cl.ExitedFilterFuncs, func(exitStatus int) bool {
		return exited == exitStatus
	})
	return nil
}

func (cl *FilterContext) foldStatusFilter(_ context.Context, filter, value string) error {
	status := containerd.ProcessStatus(value)
	switch status {
	case containerd.Running, containerd.Created, containerd.Stopped, containerd.Paused, containerd.Pausing, containerd.Unknown:
		cl.StatusFilterFuncs = append(cl.StatusFilterFuncs, func(stats containerd.ProcessStatus) bool {
			return status == stats
		})
	case containerd.ProcessStatus("exited"):
		cl.StatusFilterFuncs = append(cl.StatusFilterFuncs, func(stats containerd.ProcessStatus) bool {
			return containerd.Stopped == stats
		})
	case containerd.ProcessStatus("restarting"), containerd.ProcessStatus("removing"), containerd.ProcessStatus("dead"):
		logrus.Warnf("%s is not supported and is ignored", filter)
	default:
		return fmt.Errorf("invalid filter '%s'", filter)
	}
	return nil
}

func (cl *FilterContext) foldBeforeFilter(ctx context.Context, filter, value string) error {
	beforeC, err := IDOrNameFilter(ctx, cl.Containers, value)
	if err == nil {
		cl.BeforeFilterFuncs = append(cl.BeforeFilterFuncs, func(t time.Time) bool {
			return t.Before(beforeC.CreatedAt)
		})
	}
	return err
}

func (cl *FilterContext) foldSinceFilter(ctx context.Context, filter, value string) error {
	sinceC, err := IDOrNameFilter(ctx, cl.Containers, value)
	if err == nil {
		cl.SinceFilterFuncs = append(cl.SinceFilterFuncs, func(t time.Time) bool {
			return t.After(sinceC.CreatedAt)
		})
	}
	return err
}

func (cl *FilterContext) foldIDFilter(_ context.Context, filter, value string) error {
	cl.IDFilterFuncs = append(cl.IDFilterFuncs, func(id string) bool {
		if value == "" {
			return false
		}
		return strings.HasPrefix(id, value)
	})
	return nil
}

func (cl *FilterContext) foldNameFilter(_ context.Context, filter, value string) error {
	cl.NameFilterFuncs = append(cl.NameFilterFuncs, func(name string) bool {
		if value == "" {
			return true
		}
		return strings.Contains(name, value)
	})
	return nil
}

func (cl *FilterContext) foldLabelFilter(_ context.Context, filter, value string) error {
	k, v, hasValue := value, "", false
	if subs := strings.SplitN(value, "=", 2); len(subs) == 2 {
		hasValue = true
		k, v = subs[0], subs[1]
	}
	cl.LabelFilterFuncs = append(cl.LabelFilterFuncs, func(labels map[string]string) bool {
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

func (cl *FilterContext) foldVolumeFilter(_ context.Context, filter, value string) error {
	cl.VolumeFilterFuncs = append(cl.VolumeFilterFuncs, func(vols []*volume.ContainerVolume) bool {
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

func (cl *FilterContext) foldNetworkFilter(_ context.Context, filter, value string) error {
	cl.NetworkFilterFuncs = append(cl.NetworkFilterFuncs, func(networks []string) bool {
		for _, network := range networks {
			if network == value {
				return true
			}
		}
		return false
	})
	return nil
}

func (cl *FilterContext) matchesInfoFilters(ctx context.Context, container containerd.Container) bool {
	if len(cl.IDFilterFuncs)+len(cl.NameFilterFuncs)+len(cl.BeforeFilterFuncs)+
		len(cl.SinceFilterFuncs)+len(cl.LabelFilterFuncs)+len(cl.VolumeFilterFuncs)+len(cl.NetworkFilterFuncs) == 0 {
		return true
	}
	info, _ := container.Info(ctx, containerd.WithoutRefreshedMetadata)
	return cl.matchesIDFilter(info) && cl.matchesNameFilter(info) && cl.matchesBeforeFilter(info) &&
		cl.matchesSinceFilter(info) && cl.matchesLabelFilter(info) && cl.matchesVolumeFilter(info) &&
		cl.matchesNetworkFilter(info)
}

func (cl *FilterContext) matchesTaskFilters(ctx context.Context, container containerd.Container) bool {
	if len(cl.ExitedFilterFuncs)+len(cl.StatusFilterFuncs) == 0 {
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

func (cl *FilterContext) matchesExitedFilter(status containerd.Status) bool {
	if len(cl.ExitedFilterFuncs) == 0 {
		return true
	}
	if status.Status != containerd.Stopped {
		return false
	}
	for _, exitedFilterFunc := range cl.ExitedFilterFuncs {
		if !exitedFilterFunc(int(status.ExitStatus)) {
			continue
		}
		return true
	}
	return false
}

func (cl *FilterContext) matchesStatusFilter(status containerd.Status) bool {
	if len(cl.StatusFilterFuncs) == 0 {
		return true
	}
	for _, statusFilterFunc := range cl.StatusFilterFuncs {
		if !statusFilterFunc(status.Status) {
			continue
		}
		return true
	}
	return false
}

func (cl *FilterContext) matchesIDFilter(info containers.Container) bool {
	if len(cl.IDFilterFuncs) == 0 {
		return true
	}
	for _, idFilterFunc := range cl.IDFilterFuncs {
		if !idFilterFunc(info.ID) {
			continue
		}
		return true
	}
	return false
}

func (cl *FilterContext) matchesNameFilter(info containers.Container) bool {
	if len(cl.NameFilterFuncs) == 0 {
		return true
	}
	cName := utils.GetPrintableContainerName(info.Labels)
	for _, nameFilterFunc := range cl.NameFilterFuncs {
		if !nameFilterFunc(cName) {
			continue
		}
		return true
	}
	return false
}

func (cl *FilterContext) matchesSinceFilter(info containers.Container) bool {
	if len(cl.SinceFilterFuncs) == 0 {
		return true
	}
	for _, sinceFilterFunc := range cl.SinceFilterFuncs {
		if !sinceFilterFunc(info.CreatedAt) {
			continue
		}
		return true
	}
	return false
}

func (cl *FilterContext) matchesBeforeFilter(info containers.Container) bool {
	if len(cl.BeforeFilterFuncs) == 0 {
		return true
	}
	for _, beforeFilterFunc := range cl.BeforeFilterFuncs {
		if !beforeFilterFunc(info.CreatedAt) {
			continue
		}
		return true
	}
	return false
}

func (cl *FilterContext) matchesLabelFilter(info containers.Container) bool {
	for _, labelFilterFunc := range cl.LabelFilterFuncs {
		if !labelFilterFunc(info.Labels) {
			return false
		}
	}
	return true
}

func (cl *FilterContext) matchesVolumeFilter(info containers.Container) bool {
	if len(cl.VolumeFilterFuncs) == 0 {
		return true
	}
	vols := volume.GetContainerVolumes(info.Labels)
	for _, volumeFilterFunc := range cl.VolumeFilterFuncs {
		if !volumeFilterFunc(vols) {
			continue
		}
		return true
	}
	return false
}

func (cl *FilterContext) matchesNetworkFilter(info containers.Container) bool {
	if len(cl.NetworkFilterFuncs) == 0 {
		return true
	}
	networks := utils.GetContainerNetworks(info.Labels)
	for _, networkFilterFunc := range cl.NetworkFilterFuncs {
		if !networkFilterFunc(networks) {
			continue
		}
		return true
	}
	return false
}

func IDOrNameFilter(ctx context.Context, containers []containerd.Container, value string) (*containers.Container, error) {
	for _, container := range containers {
		info, err := container.Info(ctx, containerd.WithoutRefreshedMetadata)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(info.ID, value) || strings.Contains(utils.GetPrintableContainerName(info.Labels), value) {
			return &info, nil
		}
	}
	return nil, fmt.Errorf("no such container %s", value)
}

func GetContainerSize(ctx context.Context, client *containerd.Client, c containerd.Container, info containers.Container) (string, error) {
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

	imageSize, err := utils.UnpackedImageSize(ctx, sn, image)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s (virtual %s)", progress.Bytes(containerSize).String(), progress.Bytes(imageSize).String()), nil
}

func RemoveContainer(ctx context.Context, cmd *cobra.Command, container containerd.Container, force bool, removeAnonVolumes bool) (retErr error) {
	ns, err := namespaces.NamespaceRequired(ctx)
	if err != nil {
		return err
	}

	id := container.ID()
	l, err := container.Labels(ctx)
	if err != nil {
		return err
	}
	stateDir := l[labels.StateDir]
	name := l[labels.Name]

	dataStore, err := client.GetDataStore(cmd)
	if err != nil {
		return err
	}
	namst, err := namestore.New(dataStore, ns)
	if err != nil {
		return err
	}

	defer func() {
		if errdefs.IsNotFound(retErr) {
			retErr = nil
		}
		if retErr != nil {
			return
		}

		if err := os.RemoveAll(stateDir); err != nil {
			logrus.WithError(retErr).Warnf("failed to remove container state dir %s", stateDir)
		}
		if name != "" {
			if err := namst.Release(name, id); err != nil {
				logrus.WithError(retErr).Warnf("failed to release container Name %s", name)
			}
		}
		if err := hostsstore.DeallocHostsFile(dataStore, ns, id); err != nil {
			logrus.WithError(retErr).Warnf("failed to remove hosts file for container %q", id)
		}
	}()
	if anonVolumesJSON, ok := l[labels.AnonymousVolumes]; ok && removeAnonVolumes {
		var anonVolumes []string
		if err := json.Unmarshal([]byte(anonVolumesJSON), &anonVolumes); err != nil {
			return err
		}
		volStore, err := volume.GetVolumeStore(cmd)
		if err != nil {
			return err
		}
		defer func() {
			if _, err := volStore.Remove(anonVolumes); err != nil {
				logrus.WithError(err).Warnf("failed to remove anonymous volume %v", anonVolumes)
			}
		}()
	}

	task, err := container.Task(ctx, cio.Load)
	if err != nil {
		if errdefs.IsNotFound(err) {
			if container.Delete(ctx, containerd.WithSnapshotCleanup) != nil {
				return container.Delete(ctx)
			}
		}
		return err
	}

	status, err := task.Status(ctx)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return err
	}

	switch status.Status {
	case containerd.Created, containerd.Stopped:
		if _, err := task.Delete(ctx); err != nil && !errdefs.IsNotFound(err) {
			return fmt.Errorf("failed to delete task %v: %w", id, err)
		}
	case containerd.Paused:
		if !force {
			return common.NewStatusError(fmt.Errorf("you cannot remove a %v container %v. Unpause the container before attempting removal or force remove", status.Status, id))
		}
		_, err := task.Delete(ctx, containerd.WithProcessKill)
		if err != nil && !errdefs.IsNotFound(err) {
			return fmt.Errorf("failed to delete task %v: %w", id, err)
		}
	// default is the case, when status.Status = containerd.Running
	default:
		if !force {
			return common.NewStatusError(fmt.Errorf("you cannot remove a %v container %v. Stop the container before attempting removal or force remove", status.Status, id))
		}
		if err := task.Kill(ctx, syscall.SIGKILL); err != nil {
			logrus.WithError(err).Warnf("failed to send SIGKILL")
		}
		es, err := task.Wait(ctx)
		if err == nil {
			<-es
		}
		_, err = task.Delete(ctx, containerd.WithProcessKill)
		if err != nil && !errdefs.IsNotFound(err) {
			logrus.WithError(err).Warnf("failed to delete task %v", id)
		}
	}
	var delOpts []containerd.DeleteOpts
	if _, err := container.Image(ctx); err == nil {
		delOpts = append(delOpts, containerd.WithSnapshotCleanup)
	}

	if err := container.Delete(ctx, delOpts...); err != nil {
		return err
	}
	return err
}
