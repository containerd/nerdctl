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

func usedVolumes(ctx context.Context, containers []containerd.Container) (map[string]struct{}, error) {
	usedVolumesList := make(map[string]struct{})
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
				usedVolumesList[m.Name] = struct{}{}
			}
		}
	}
	return usedVolumesList, nil
}
