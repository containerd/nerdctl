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

package volumestore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/identifiers"
	"github.com/containerd/nerdctl/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/pkg/lockutil"
	"github.com/containerd/nerdctl/pkg/strutil"
)

// Path returns a string like `/var/lib/nerdctl/1935db59/volumes/default`.
func Path(dataStore, ns string) (string, error) {
	if dataStore == "" || ns == "" {
		return "", errdefs.ErrInvalidArgument
	}
	volStore := filepath.Join(dataStore, "volumes", ns)
	return volStore, nil
}

// New returns a VolumeStore
func New(dataStore, ns string) (VolumeStore, error) {
	volStoreDir, err := Path(dataStore, ns)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(volStoreDir, 0700); err != nil {
		return nil, err
	}
	vs := &volumeStore{
		dir: volStoreDir,
	}
	return vs, nil
}

// DataDirName is "_data"
const DataDirName = "_data"

const volumeJSONFileName = "volume.json"

type VolumeStore interface {
	Dir() string
	Create(name string, labels []string) (*native.Volume, error)
	// Get may return ErrNotFound
	Get(name string) (*native.Volume, error)
	List() (map[string]native.Volume, error)
	Remove(names []string) (removedNames []string, err error)
}

type volumeStore struct {
	// dir is a string like `/var/lib/nerdctl/1935db59/volumes/default`.
	// dir is guaranteed to exist.
	dir string
}

func (vs *volumeStore) Dir() string {
	return vs.dir
}

func (vs *volumeStore) Create(name string, labels []string) (*native.Volume, error) {
	if err := identifiers.Validate(name); err != nil {
		return nil, fmt.Errorf("malformed name %s: %w", name, err)
	}
	volPath := filepath.Join(vs.dir, name)
	volDataPath := filepath.Join(volPath, DataDirName)
	fn := func() error {
		if err := os.Mkdir(volPath, 0700); err != nil {
			return err
		}
		if err := os.Mkdir(volDataPath, 0755); err != nil {
			return err
		}

		type volumeOpts struct {
			Labels map[string]string `json:"labels"`
		}

		labelsMap := strutil.ConvertKVStringsToMap(labels)

		volOpts := volumeOpts{
			Labels: labelsMap,
		}

		labelsJSON, err := json.MarshalIndent(volOpts, "", "    ")
		if err != nil {
			return err
		}

		volFilePath := filepath.Join(volPath, volumeJSONFileName)
		if err := os.WriteFile(volFilePath, labelsJSON, 0644); err != nil {
			return err
		}
		return nil
	}

	if err := lockutil.WithDirLock(vs.dir, fn); err != nil {
		return nil, err
	}

	vol := &native.Volume{
		Name:       name,
		Mountpoint: volDataPath,
	}
	return vol, nil
}

func (vs *volumeStore) Get(name string) (*native.Volume, error) {
	if err := identifiers.Validate(name); err != nil {
		return nil, fmt.Errorf("malformed name %s: %w", name, err)
	}
	dataPath := filepath.Join(vs.dir, name, DataDirName)
	if _, err := os.Stat(dataPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("volume %q not found: %w", name, errdefs.ErrNotFound)
		}
		return nil, err
	}

	volFilePath := filepath.Join(vs.dir, name, volumeJSONFileName)
	volumeDataBytes, err := os.ReadFile(volFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			//volume.json does not exists should not be blocking for inspect operation
		} else {
			return nil, err
		}
	}

	entry := native.Volume{
		Name:       name,
		Mountpoint: dataPath,
		Labels:     Labels(volumeDataBytes),
	}
	return &entry, nil
}

func (vs *volumeStore) List() (map[string]native.Volume, error) {
	dEnts, err := os.ReadDir(vs.dir)
	if err != nil {
		return nil, err
	}

	res := make(map[string]native.Volume, len(dEnts))
	for _, dEnt := range dEnts {
		name := dEnt.Name()
		vol, err := vs.Get(name)
		if err != nil {
			return res, err
		}
		res[name] = *vol
	}
	return res, nil
}

func (vs *volumeStore) Remove(names []string) ([]string, error) {
	var removed []string
	fn := func() error {
		for _, name := range names {
			if err := identifiers.Validate(name); err != nil {
				return fmt.Errorf("malformed name %s: %w", name, err)
			}
			dir := filepath.Join(vs.dir, name)
			if err := os.RemoveAll(dir); err != nil {
				return err
			}
			removed = append(removed, name)
		}
		return nil
	}
	err := lockutil.WithDirLock(vs.dir, fn)
	return removed, err
}

func Labels(b []byte) *map[string]string {
	type volumeOpts struct {
		Labels *map[string]string `json:"labels,omitempty"`
	}
	var vo volumeOpts
	if err := json.Unmarshal(b, &vo); err != nil {
		return nil
	}
	return vo.Labels
}
