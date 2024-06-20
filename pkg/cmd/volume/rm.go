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

	"github.com/containerd/containerd"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/mountutil"
)

func Remove(ctx context.Context, client *containerd.Client, volumes []string, options types.VolumeRemoveOptions) error {
	containers, err := client.Containers(ctx)
	if err != nil {
		return err
	}
	volStore, err := Store(options.GOptions.Namespace, options.GOptions.DataRoot, options.GOptions.Address)
	if err != nil {
		return err
	}
	usedVolumes, err := usedVolumes(ctx, containers)
	if err != nil {
		return err
	}

	var volumenames []string // nolint: prealloc
	for _, name := range volumes {
		if _, ok := usedVolumes[name]; ok {
			return fmt.Errorf("volume %q is in use", name)
		}
		volumenames = append(volumenames, name)
	}
	// if err is set, this is a hard filesystem error
	removedNames, warns, err := volStore.Remove(volumenames)
	if err != nil {
		return err
	}
	// Otherwise, output on stdout whatever was successful
	for _, name := range removedNames {
		fmt.Fprintln(options.Stdout, name)
	}
	// Log the rest
	for _, volErr := range warns {
		log.G(ctx).Warn(volErr)
	}
	if len(warns) > 0 {
		return errors.New("some volumes could not be removed")
	}
	return nil
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
