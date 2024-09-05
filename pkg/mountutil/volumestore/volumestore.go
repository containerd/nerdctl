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

// Package volumestore allows manipulating containers' volumes.
// All methods are safe to use concurrently (and perform atomic writes), except CreateWithoutLock, which is specifically
// meant to be used multiple times, inside a Lock-ed section.
package volumestore

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/identifiers"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/v2/pkg/store"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
)

const (
	volumeDirBasename  = "volumes"
	dataDirName        = "_data"
	volumeJSONFileName = "volume.json"
)

// ErrNameStore will wrap all errors here
var ErrVolumeStore = errors.New("volume-store error")

type VolumeStore interface {
	// Exists checks if a given volume exists
	Exists(name string) (bool, error)
	// Get returns an existing volume
	Get(name string, size bool) (*native.Volume, error)
	// Create will either return an existing volume, or create a new one
	// NOTE that different labels will NOT create a new volume if there is one by that name already,
	// but instead return the existing one with the (possibly different) labels
	Create(name string, labels []string) (vol *native.Volume, err error)
	// List returns all existing volumes.
	// Note that list is expensive as it reads all volumes individual info
	List(size bool) (map[string]native.Volume, error)
	// Remove one of more volumes
	Remove(generator func() ([]string, []error, error)) (removed []string, warns []error, err error)
	// Prune will call a filtering function expected to return the volumes name to delete
	Prune(filter func(volumes []*native.Volume) ([]string, error)) (err error)
	// Count returns the number of volumes
	Count() (count int, err error)

	// Lock: see store implementation
	Lock() error
	// CreateWithoutLock will create a volume (or return an existing one).
	// This method does NOT lock (unlike Create).
	// It is meant to be used between `Lock` and `Release`, and is specifically useful when multiple different volume
	// creation will have to happen in different method calls (eg: container create).
	CreateWithoutLock(name string, labels []string) (*native.Volume, error)
	// Release: see store implementation
	Release() error
}

// New returns a VolumeStore
func New(dataStore, namespace string) (volStore VolumeStore, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrVolumeStore, err)
		}
	}()

	if dataStore == "" || namespace == "" {
		return nil, store.ErrInvalidArgument
	}

	st, err := store.New(filepath.Join(dataStore, volumeDirBasename, namespace), 0, 0o644)
	if err != nil {
		return nil, err
	}

	return &volumeStore{
		Locker:  st,
		manager: st,
	}, nil
}

type volumeStore struct {
	// Expose the lock primitives directly to satisfy interface for Lock and Release
	store.Locker

	manager store.Manager
}

// Exists checks if a volume exists in the store
func (vs *volumeStore) Exists(name string) (doesExist bool, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrVolumeStore, err)
		}
	}()

	if err = identifiers.ValidateDockerCompat(name); err != nil {
		return false, err
	}

	// No need for a lock here, the operation is atomic
	return vs.manager.Exists(name)
}

// Get retrieves a native volume from the store, optionally with its size
func (vs *volumeStore) Get(name string, size bool) (vol *native.Volume, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrVolumeStore, err)
		}
	}()

	if err = identifiers.ValidateDockerCompat(name); err != nil {
		return nil, err
	}

	// If we require the size, this is no longer atomic, so, we need to lock
	err = vs.WithLock(func() error {
		vol, err = vs.rawGet(name, size)
		return err
	})

	return vol, err
}

// CreateWithoutLock will create a new volume, or return an existing one if there is one already by that name
// It does NOT lock for you - unlike all the other methods - though it *will* error if you do not lock.
// This is on purpose as volume creation in most cases are done during container creation,
// and implies an extended period of time for locking.
// To use:
// volStore.Lock()
// defer volStore.Release()
// volStore.CreateWithoutLock(...)
func (vs *volumeStore) CreateWithoutLock(name string, labels []string) (vol *native.Volume, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrVolumeStore, err)
		}
	}()

	if err = identifiers.ValidateDockerCompat(name); err != nil {
		return nil, err
	}

	return vs.rawCreate(name, labels)
}

func (vs *volumeStore) Create(name string, labels []string) (vol *native.Volume, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrVolumeStore, err)
		}
	}()

	if err = identifiers.ValidateDockerCompat(name); err != nil {
		return nil, err
	}

	err = vs.Locker.WithLock(func() error {
		vol, err = vs.rawCreate(name, labels)
		return err
	})

	return vol, err
}

func (vs *volumeStore) Count() (count int, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrVolumeStore, err)
		}
	}()

	err = vs.Locker.WithLock(func() error {
		names, err := vs.manager.List()
		if err != nil {
			return err
		}

		count = len(names)
		return nil
	})

	return count, err
}

func (vs *volumeStore) List(size bool) (res map[string]native.Volume, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrVolumeStore, err)
		}
	}()

	res = make(map[string]native.Volume)

	err = vs.Locker.WithLock(func() error {
		names, err := vs.manager.List()
		if err != nil {
			return err
		}

		for _, name := range names {
			vol, err := vs.rawGet(name, size)
			if err != nil {
				log.L.WithError(err).Errorf("something is wrong with %q", name)
				continue
			}
			res[name] = *vol
		}

		return nil
	})

	return res, err
}

// Remove will remove one or more containers
func (vs *volumeStore) Remove(generator func() ([]string, []error, error)) (removed []string, warns []error, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrVolumeStore, err)
		}
	}()

	err = vs.Locker.WithLock(func() error {
		var names []string
		names, warns, err = generator()
		if err != nil {
			return err
		}

		for _, name := range names {
			// Invalid name, soft error
			if err = identifiers.ValidateDockerCompat(name); err != nil {
				// TODO: we are clearly mixing presentation concerns here
				// This should be handled by the cli, not here
				warns = append(warns, err)
				continue
			}

			// Erroring on Exists is a hard error
			// !doesExist is a soft error
			// Inability to delete is a hard error
			if doesExist, err := vs.manager.Exists(name); err != nil {
				return err
			} else if !doesExist {
				// TODO: see above
				warns = append(warns, fmt.Errorf("volume %q: %w", name, store.ErrNotFound))
				continue
			} else if err = vs.manager.Delete(name); err != nil {
				return err
			}

			// Otherwise, add it the list of successfully removed
			removed = append(removed, name)
		}

		return nil
	})

	return removed, warns, err
}

func (vs *volumeStore) Prune(filter func(vol []*native.Volume) ([]string, error)) (err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrVolumeStore, err)
		}
	}()

	return vs.Locker.WithLock(func() error {
		names, err := vs.manager.List()
		if err != nil {
			return err
		}

		res := []*native.Volume{}
		for _, name := range names {
			vol, err := vs.rawGet(name, false)
			if err != nil {
				log.L.WithError(err).Errorf("something is wrong with %q", name)
				continue
			}
			res = append(res, vol)
		}

		toDelete, err := filter(res)
		if err != nil {
			return err
		}

		for _, name := range toDelete {
			err = vs.manager.Delete(name)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

func (vs *volumeStore) rawGet(name string, size bool) (vol *native.Volume, err error) {
	content, err := vs.manager.Get(name, volumeJSONFileName)
	if err != nil {
		return nil, err
	}

	vol = &native.Volume{
		Name:   name,
		Labels: labels(content),
	}

	vol.Mountpoint, err = vs.manager.Location(name, dataDirName)
	if err != nil {
		return nil, err
	}

	if size {
		vol.Size, err = vs.manager.GroupSize(name, dataDirName)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("failed reading volume size for %q", name), err)
		}
	}

	return vol, nil
}

func (vs *volumeStore) rawCreate(name string, labels []string) (vol *native.Volume, err error) {
	volOpts := struct {
		Labels map[string]string `json:"labels"`
	}{}

	if len(labels) > 0 {
		volOpts.Labels = strutil.ConvertKVStringsToMap(labels)
	}

	// Failure here must exit, no need to clean-up
	labelsJSON, err := json.MarshalIndent(volOpts, "", "    ")
	if err != nil {
		return nil, err
	}

	if doesExist, err := vs.manager.Exists(name, volumeJSONFileName); err != nil {
		return nil, err
	} else if !doesExist {
		if err = vs.manager.Set(labelsJSON, name, volumeJSONFileName); err != nil {
			return nil, err
		}
	} else {
		log.L.Warnf("volume %q already exists and will be returned as-is", name)
		// FIXME: we do not check if the existing volume has the same labels as requested - should we?
	}

	// At this point, we either have an existing volume, or created a new one successfully
	vol = &native.Volume{
		Name: name,
	}

	if err = vs.manager.GroupEnsure(name, dataDirName); err != nil {
		return nil, err
	}

	if vol.Mountpoint, err = vs.manager.Location(name, dataDirName); err != nil {
		return nil, err
	}

	return vol, nil
}

// Private helpers
func labels(b []byte) *map[string]string {
	type volumeOpts struct {
		Labels *map[string]string `json:"labels,omitempty"`
	}
	var vo volumeOpts
	if err := json.Unmarshal(b, &vo); err != nil {
		return nil
	}
	return vo.Labels
}
