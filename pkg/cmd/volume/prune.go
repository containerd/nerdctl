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
	"fmt"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/labels"
)

func Prune(ctx context.Context, client *containerd.Client, options types.VolumePruneOptions) error {
	// Get the volume store and lock it until we are done.
	// This will prevent racing new containers from being created or removed until we are done with the cleanup of volumes
	volStore, err := Store(options.GOptions.Namespace, options.GOptions.DataRoot, options.GOptions.Address)
	if err != nil {
		return err
	}
	err = volStore.Lock()
	if err != nil {
		return err
	}
	defer volStore.Unlock()

	// Get containers and see which volumes are used
	containers, err := client.Containers(ctx)
	if err != nil {
		return err
	}

	usedVolumesList, err := usedVolumes(ctx, containers)
	if err != nil {
		return err
	}
	var removeNames []string // nolint: prealloc

	// Get the list of known volumes from the store
	volumes, err := volStore.List(false)
	if err != nil {
		return err
	}

	// Iterate through the known volumes, making sure we do not remove in-use volumes
	// but capture as well anon volumes (if --all was passed)
	for _, volume := range volumes {
		if _, ok := usedVolumesList[volume.Name]; ok {
			continue
		}
		if !options.All {
			if volume.Labels == nil {
				continue
			}
			val, ok := (*volume.Labels)[labels.AnonymousVolumes]
			//skip the named volume and only remove the anonymous volume
			if !ok || val != "" {
				continue
			}
		}
		removeNames = append(removeNames, volume.Name)
	}

	// Remove the volumes from that list
	removedNames, _, err := volStore.Remove(removeNames)
	if err != nil {
		return err
	}
	if len(removedNames) > 0 {
		fmt.Fprintln(options.Stdout, "Deleted Volumes:")
		for _, name := range removedNames {
			fmt.Fprintln(options.Stdout, name)
		}
		fmt.Fprintln(options.Stdout, "")
	}
	return nil
}
