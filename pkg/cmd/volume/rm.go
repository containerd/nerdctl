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
	"errors"
	"fmt"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/mountutil"
)

func Remove(ctx context.Context, client *containerd.Client, volumes []string, options types.VolumeRemoveOptions) error {
	volStore, err := Store(options.GOptions.Namespace, options.GOptions.DataRoot, options.GOptions.Address)
	if err != nil {
		return err
	}

	containers, err := client.Containers(ctx)
	if err != nil {
		return err
	}

	// Note: to avoid racy behavior, this is called by volStore.Remove *inside a lock*
	removableVolumes := func() (volumeNames []string, cannotRemove []error, err error) {
		usedVolumesList, err := usedVolumes(ctx, containers)
		if err != nil {
			return nil, nil, err
		}

		for _, name := range volumes {
			if _, ok := usedVolumesList[name]; ok {
				cannotRemove = append(cannotRemove, fmt.Errorf("volume %q is in use (%w)", name, errdefs.ErrFailedPrecondition))
				continue
			}
			volumeNames = append(volumeNames, name)
		}

		return volumeNames, cannotRemove, nil
	}

	removedNames, cannotRemove, err := volStore.Remove(removableVolumes)
	if err != nil {
		return err
	}
	// Otherwise, output on stdout whatever was successful
	for _, name := range removedNames {
		fmt.Fprintln(options.Stdout, name)
	}
	// Log the rest
	for _, volErr := range cannotRemove {
		log.G(ctx).Warn(volErr)
	}
	if len(cannotRemove) > 0 {
		return errors.New("some volumes could not be removed")
	}
	return nil
}

// usedVolumes returns, per volume name, how many containers mount it. Callers that only care about
// whether a volume is used at all can test for the presence of the key.
func usedVolumes(ctx context.Context, containers []containerd.Container) (map[string]int64, error) {
	usedVolumesList := make(map[string]int64)
	for _, c := range containers {
		l, err := c.Labels(ctx)
		if err != nil {
			// Containerd note: there is no guarantee that the containers we got from the list still exist at this point
			// If that is the case, just ignore and move on
			if errors.Is(err, errdefs.ErrNotFound) {
				log.G(ctx).Debugf("container %q is gone - ignoring", c.ID())
				continue
			}
			return nil, err
		}

		names, err := mountedVolumes(labels.GetMount(l))
		if err != nil {
			return nil, err
		}
		for name := range names {
			usedVolumesList[name]++
		}
	}
	return usedVolumesList, nil
}

// mountedVolumes returns the distinct volume names of a container, from the JSON-marshalled mounts
// it carries in its labels. The names are deduplicated: a container mounting the same volume at
// several paths is still one reference to it, which is how Docker counts the links of a volume.
func mountedVolumes(mountsJSON string) (map[string]struct{}, error) {
	names := make(map[string]struct{})
	if mountsJSON == "" {
		return names, nil
	}

	var mounts []dockercompat.MountPoint
	if err := json.Unmarshal([]byte(mountsJSON), &mounts); err != nil {
		return nil, err
	}
	for _, m := range mounts {
		if m.Type == mountutil.Volume {
			names[m.Name] = struct{}{}
		}
	}
	return names, nil
}
