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
	"encoding/json"

	"github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/mountutil/volumestore"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// ContainerVolume is a misnomer.
// TODO: find a better name.
type ContainerVolume struct {
	Type        string
	Name        string
	Source      string
	Destination string
	Mode        string
	RW          bool
	Propagation string
}

// Store returns a volume store
// that corresponds to a directory like `/var/lib/nerdctl/1935db59/volume/default`
func Store(cmd *cobra.Command) (volumestore.VolumeStore, error) {
	ns, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return nil, err
	}
	dataStore, err := client.DataStore(cmd)
	if err != nil {
		return nil, err
	}
	return volumestore.New(dataStore, ns)
}

func Volumes(cmd *cobra.Command) (map[string]native.Volume, error) {
	volStore, err := Store(cmd)
	if err != nil {
		return nil, err
	}
	volumeSize, err := cmd.Flags().GetBool("size")
	if err != nil {
		return nil, err
	}
	return volStore.List(volumeSize)
}

// ContainerVolumes is a misnomer.
// TODO: find a better name.
func ContainerVolumes(containerLabels map[string]string) []*ContainerVolume {
	var vols []*ContainerVolume
	volLabels := []string{labels.AnonymousVolumes, labels.Mounts}
	for _, volLabel := range volLabels {
		names, ok := containerLabels[volLabel]
		if !ok {
			continue
		}
		var (
			volumes []*ContainerVolume
			err     error
		)
		if volLabel == labels.Mounts {
			err = json.Unmarshal([]byte(names), &volumes)
		}
		if volLabel == labels.AnonymousVolumes {
			var anonymous []string
			err = json.Unmarshal([]byte(names), &anonymous)
			for _, anony := range anonymous {
				volumes = append(volumes, &ContainerVolume{Name: anony})
			}

		}
		if err != nil {
			logrus.Warn(err)
		}
		vols = append(vols, volumes...)
	}
	return vols
}
