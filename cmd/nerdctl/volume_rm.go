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

package main

import (
	"encoding/json"
	"fmt"

	"github.com/containerd/nerdctl/pkg/containerinspector"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newVolumeRmCommand() *cobra.Command {
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
	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()
	containers, err := client.Containers(ctx)
	if err != nil {
		return err
	}
	var mount *specs.Spec
	var volumenames []string
	volStore, err := getVolumeStore(cmd)
	if err != nil {
		return err
	}
	names := args

	for _, name := range names {
		var found bool
		for _, container := range containers {
			n, err := containerinspector.Inspect(ctx, container)
			if err != nil {
				return err
			}
			err = json.Unmarshal(n.Container.Spec.Value, &mount)
			if err != nil {
				return err
			}
			volume, _ := volStore.Get(name)
			if found = checkVolume(&mount.Mounts, volume.Mountpoint); found {
				found = true
				logrus.WithError(fmt.Errorf("Volume %q is in use", name)).Error(fmt.Errorf("Remove Volume: %q failed", name))
				break
			}
		}
		if !found {
			// volumes that are to be deleted
			volumenames = append(volumenames, name)
		}
	}
	if volumenames != nil {
		volnames, err := volStore.Remove(volumenames)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), volnames)
	}
	return err
}

func volumeRmShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show volume names
	return shellCompleteVolumeNames(cmd)
}

// checkVolume checks whether the container mount path and the given volume mount point are same or not.
func checkVolume(mounts *[]specs.Mount, volmountpoint string) bool {
	for _, mount := range *mounts {
		if mount.Source == volmountpoint {
			return true
		}
	}
	return false
}
