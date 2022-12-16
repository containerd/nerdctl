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

package volume

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/containerd/containerd"
	ncclient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/volume"
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/mountutil"
	"github.com/spf13/cobra"
)

func NewRmCommand() *cobra.Command {
	volumeRmCommand := &cobra.Command{
		Use:               "rm [flags] VOLUME [VOLUME...]",
		Aliases:           []string{"remove"},
		Short:             "Remove one or more volumes",
		Long:              "NOTE: You cannot remove a volume that is in use by a container.",
		Args:              cobra.MinimumNArgs(1),
		RunE:              volumeRmAction,
		ValidArgsFunction: volumeRmShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	volumeRmCommand.Flags().BoolP("force", "f", false, "(unimplemented yet)")
	return volumeRmCommand
}

func volumeRmAction(cmd *cobra.Command, args []string) error {
	client, ctx, cancel, err := ncclient.New(cmd)
	if err != nil {
		return err
	}
	defer cancel()
	containers, err := client.Containers(ctx)
	if err != nil {
		return err
	}
	volStore, err := volume.GetVolumeStore(cmd)
	if err != nil {
		return err
	}
	names := args
	usedVolumes, err := usedVolumes(ctx, containers)
	if err != nil {
		return err
	}

	var volumenames []string // nolint: prealloc
	for _, name := range names {
		volume, err := volStore.Get(name, false)
		if err != nil {
			return err
		}
		if _, ok := usedVolumes[volume.Name]; ok {
			return fmt.Errorf("volume %q is in use", name)
		}
		volumenames = append(volumenames, name)
	}
	removedNames, err := volStore.Remove(volumenames)
	if err != nil {
		return err
	}
	for _, name := range removedNames {
		fmt.Fprintln(cmd.OutOrStdout(), name)
	}
	return err
}

func volumeRmShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show volume names
	return completion.ShellCompleteVolumeNames(cmd)
}

func usedVolumes(ctx context.Context, containers []containerd.Container) (map[string]struct{}, error) {
	usedVolumes := make(map[string]struct{})
	for _, c := range containers {
		l, err := c.Labels(ctx)
		if err != nil {
			return nil, err
		}
		mountsJSON, ok := l[labels.Mounts]
		if !ok {
			continue
		}

		var mounts []dockercompat.MountPoint
		err = json.Unmarshal([]byte(mountsJSON), &mounts)
		if err != nil {
			return nil, err
		}
		for _, m := range mounts {
			if m.Type == mountutil.Volume {
				usedVolumes[m.Name] = struct{}{}
			}
		}
	}
	return usedVolumes, nil
}
