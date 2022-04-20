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
	"log"
	"strings"

	"github.com/containerd/nerdctl/pkg/containerinspector"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newVolumePruneCommand() *cobra.Command {
	volumePruneCommand := &cobra.Command{
		Use:           "prune",
		Short:         "Remove all unused local volumes. Unused local volumes are those which are not referenced by any containers.",
		Long:          "NOTE: volume in use is not deleted",
		RunE:          volumePruneAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	volumePruneCommand.Flags().BoolP("force", "f", false, "Do not prompt for confirmation")
	volumePruneCommand.Flags().StringArray("filter", nil, "Provide filter values (e.g. 'label=<label>')")
	return volumePruneCommand
}

func volumePruneAction(cmd *cobra.Command, args []string) error {
	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}
	if !force {
		var confirm string
		log.Println("check")
		fmt.Fprintf(cmd.OutOrStdout(), "%s", "WARNING! This will remove all local volumes not used by at least one container.\nAre you sure you want to continue? [y/N] ")
		fmt.Fscanf(cmd.InOrStdin(), "%s", &confirm)

		if strings.ToLower(confirm) != "y" {
			return nil
		}
	}
	filterMaps, err := checkVolFilter(cmd)
	if err != nil {
		return err
	}
	containers, err := client.Containers(ctx)
	if err != nil {
		return err
	}
	var mount *specs.Spec
	var removedNames []string
	volumes, err := getVolumes(cmd)
	if err != nil {
		return err
	}
	volStore, err := getVolumeStore(cmd)
	if err != nil {
		return err
	}
	for name, volDetails := range volumes {
		var found bool
		if filterMaps != nil {
			ok := getFilteredVolumes(*volDetails.Labels, filterMaps)
			if !ok {
				continue
			}
		}
		for _, container := range containers {
			n, err := containerinspector.Inspect(ctx, container)
			if err != nil {
				return err
			}
			err = json.Unmarshal(n.Container.Spec.GetValue(), &mount)
			if err != nil {
				return err
			}
			if found = checkVolume(&mount.Mounts, volDetails.Mountpoint); found {
				found = true
				logrus.WithError(fmt.Errorf("Volume %q is in use", name)).Error(fmt.Errorf("Remove Volume: %q failed", name))
				break
			}
		}
		if !found {
			// volumes that are not mounted to any container
			removedNames = append(removedNames, name)
		}
	}
	if removedNames != nil {
		rNames, err := volStore.Remove(removedNames)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Deleted Volumes:")
		for _, elem := range rNames {
			fmt.Fprintln(cmd.OutOrStdout(), elem)
		}
	}
	return nil
}
func checkVolFilter(cmd *cobra.Command) (map[string]string, error) {
	filters, err := cmd.Flags().GetStringArray("filter")
	if err != nil {
		return nil, err
	}
	filters = strutil.DedupeStrSlice(filters)
	filtersMap := strutil.ConvertKVStringsToMap(filters)
	if err != nil {
		return nil, err
	}
	return filtersMap, nil
}

func getFilteredVolumes(labels map[string]string, filterMaps map[string]string) bool {
	for _, key := range filterMaps {
		if _, ok := labels[key]; ok {
			return true
		}
	}
	return false
}
