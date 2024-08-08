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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/containerd/containerd/identifiers"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/v2/pkg/lockutil"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
	"github.com/opencontainers/go-digest"
)

const (
	dataDirName        = "_data"
	volumeJSONFileName = "volume.json"
)

var (
	validateRegExp = regexp.MustCompile("^[a-zA-Z0-9_+:/-]{1,256}$")
)

func validateName(name string) error {
	if !validateRegExp.MatchString(name) {
		return fmt.Errorf("identifier %q must match %v: %w", name, validateRegExp, errdefs.ErrInvalidArgument)
	}
	return nil
}

func validateLegacyName(name string) error {
	return identifiers.Validate(name)
}

// VolumeStore allows manipulating containers' volumes
// Every method is protected by a file lock, and is safe to use concurrently.
// If you need to use multiple methods successively (for example: List, then Remove), you should instead optin
// for an explicit durable lock, by first calling `Lock` then `defer Unlock`.
// This is also true (and important to do) for any operation that is going to inspect containers before going for
// creation or removal of volumes.
type VolumeStore interface {
	Create(name string, labels []string) (*native.Volume, error)
	Get(name string, size bool) (*native.Volume, error)
	List(size bool) (map[string]native.Volume, error)
	Remove(names []string) (removed []string, warns []error, err error)
	Lock() error
	Unlock() error
}

// New returns a VolumeStore
func New(dataStore, ns string) (VolumeStore, error) {
	if dataStore == "" || ns == "" {
		return nil, errdefs.ErrInvalidArgument
	}
	volStoreDir := filepath.Join(dataStore, "volumes", ns)

	if err := os.MkdirAll(volStoreDir, 0700); err != nil {
		return nil, err
	}
	vs := &volumeStore{
		dir: volStoreDir,
	}
	return vs, nil
}

type volumeStore struct {
	dir    string
	locked *os.File
}

// Lock should be called when you need an exclusive lock on the volume store for an extended period of time
// spanning multiple atomic method calls.
// Be sure to defer Unlock to release it.
func (vs *volumeStore) Lock() error {
	if vs.locked != nil {
		return fmt.Errorf("cannot lock already locked volume store %q", vs.dir)
	}

	dirFile, err := lockutil.Lock(vs.dir)
	if err != nil {
		return err
	}

	vs.locked = dirFile
	return nil
}

// Unlock should be called once done (see Lock) to release the persistent lock on the store
func (vs *volumeStore) Unlock() error {
	if vs.locked == nil {
		return fmt.Errorf("cannot unlock already unlocked volume store %q", vs.dir)
	}

	defer func() {
		vs.locked = nil
	}()

	if err := lockutil.Unlock(vs.locked); err != nil {
		return fmt.Errorf("failed to unlock volume store %q: %w", vs.dir, err)
	}
	return nil
}

func (vs *volumeStore) legacyEnsureLink(name string) error {
	legacyVolPath := filepath.Join(vs.dir, name)
	if _, err := os.Stat(legacyVolPath); err == nil {
		volPath := filepath.Join(vs.dir, strings.Replace(digest.FromString(name).String(), ":", "_", -1))
		if _, err = os.Stat(volPath); errors.Is(err, os.ErrNotExist) {
			// Ensure we have a name inside the marker file
			volFilePath := filepath.Join(legacyVolPath, volumeJSONFileName)
			volOpts := struct {
				Name   string            `json:"name"`
				Labels map[string]string `json:"labels"`
			}{}
			volumeDataBytes, err := os.ReadFile(volFilePath)
			if err != nil {
				return err
			}
			if err = json.Unmarshal(volumeDataBytes, &volOpts); err != nil {
				return err
			}
			volOpts.Name = name
			labelsJSON, err := json.MarshalIndent(volOpts, "", "    ")
			if err != nil {
				return err
			}
			if err = os.WriteFile(volFilePath, labelsJSON, 0644); err != nil {
				return err
			}

			// Symlink
			err = os.Symlink(legacyVolPath, volPath)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Create will create a new volume, or return an existing one if there is one already by that name
// Besides a possible locking error, it might return ErrInvalidArgument, hard filesystem errors, json errors
func (vs *volumeStore) Create(name string, labels []string) (*native.Volume, error) {
	if err := validateName(name); err != nil {
		return nil, fmt.Errorf("malformed volume name: %w (%w)", err, errdefs.ErrInvalidArgument)
	}
	volPath := filepath.Join(vs.dir, strings.Replace(digest.FromString(name).String(), ":", "_", -1))
	volDataPath := filepath.Join(volPath, dataDirName)
	volFilePath := filepath.Join(volPath, volumeJSONFileName)

	vol := &native.Volume{}

	fn := func() error {
		// If there is a legacy volume hanging around, link it
		if validateLegacyName(name) == nil {
			if err := vs.legacyEnsureLink(name); err != nil {
				return err
			}
		}

		// Failures that are not os.ErrExist must exit here
		if err := os.Mkdir(volPath, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		if err := os.Mkdir(volDataPath, 0755); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}

		// If it does not exist
		if _, err := os.Stat(volFilePath); err != nil {
			// Any other stat error than "not exists", hard exit
			if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			// Error was does not exist, so, write it
			volOpts := struct {
				Name   string            `json:"name"`
				Labels map[string]string `json:"labels"`
			}{
				Name: name,
			}

			if len(labels) > 0 {
				volOpts.Labels = strutil.ConvertKVStringsToMap(labels)
			}

			// Failure here must exit, no need to clean-up
			labelsJSON, err := json.MarshalIndent(volOpts, "", "    ")
			if err != nil {
				return err
			}

			if err = os.WriteFile(volFilePath, labelsJSON, 0644); err != nil {
				return err
			}
		} else {
			log.L.Warnf("volume %q already exists and will be returned as-is", name)
		}

		// At this point, we either have a volume, or created a new one successfully
		vol.Name = name
		vol.Mountpoint = volDataPath

		return nil
	}

	var err error
	if vs.locked == nil {
		err = lockutil.WithDirLock(vs.dir, fn)
	} else {
		err = fn()
	}
	if err != nil {
		return nil, err
	}

	return vol, nil
}

// Get retrieves a native volume from the store
// Besides a possible locking error, it might return ErrInvalidArgument, ErrNotFound, or a filesystem error
func (vs *volumeStore) Get(nameOrDigest string, size bool) (*native.Volume, error) {
	name := ""
	var dgst string

	if _, err := digest.Parse(strings.Replace(nameOrDigest, "_", ":", -1)); err != nil {
		name = nameOrDigest
		if err = validateName(name); err != nil {
			return nil, fmt.Errorf("malformed volume name %q: %w", name, err)
		}
		dgst = strings.Replace(digest.FromString(name).String(), ":", "_", -1)
	} else {
		dgst = nameOrDigest
	}

	volPath := filepath.Join(vs.dir, dgst)
	volDataPath := filepath.Join(volPath, dataDirName)
	volFilePath := filepath.Join(volPath, volumeJSONFileName)

	vol := &native.Volume{}

	fn := func() error {
		// Ensure to legacy link if necessary
		if name != "" {
			if validateLegacyName(name) == nil {
				if err := vs.legacyEnsureLink(name); err != nil {
					return err
				}
			}
		}

		if _, err := os.Stat(volDataPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("%q does not exist in the volume store: %w", nameOrDigest, errdefs.ErrNotFound)
			}
			return fmt.Errorf("filesystem error reading %q from the volume store: %w", nameOrDigest, err)
		}

		volumeDataBytes, err := os.ReadFile(volFilePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("%q labels file does not exist in the volume store: %w", nameOrDigest, errdefs.ErrNotFound)
			}
			return fmt.Errorf("filesystem error reading %q from the volume store: %w", nameOrDigest, err)
		}

		// Unmarshall labels and name
		if err = json.Unmarshal(volumeDataBytes, &vol); err != nil {
			return err
		}

		vol.Mountpoint = volDataPath

		if size {
			vol.Size, err = volumeSize(vol)
			if err != nil {
				return fmt.Errorf("failed reading volume size for %q from the volume store: %w", vol.Name, err)
			}
		}
		return nil
	}

	var err error
	if vs.locked == nil {
		err = lockutil.WithDirLock(vs.dir, fn)
	} else {
		err = fn()
	}
	if err != nil {
		return nil, err
	}

	return vol, nil
}

// List retrieves all known volumes from the store.
// Besides a possible locking error, it might return ErrNotFound (indicative that the store is in a broken state), or a filesystem error
func (vs *volumeStore) List(size bool) (map[string]native.Volume, error) {
	res := map[string]native.Volume{}

	fn := func() error {
		dirEntries, err := os.ReadDir(vs.dir)
		if err != nil {
			return fmt.Errorf("filesystem error while trying to list volumes from the volume store: %w", err)
		}

		for _, dirEntry := range dirEntries {
			name := dirEntry.Name()
			vol, err := vs.Get(name, size)
			if err != nil {
				return err
			}
			res[vol.Name] = *vol
		}
		return nil
	}

	var err error
	// Since we are calling Get, we need to acquire a global lock
	if vs.locked == nil {
		err = vs.Lock()
		if err != nil {
			return nil, err
		}
		defer vs.Unlock()
	}
	err = fn()
	if err != nil {
		return nil, err
	}
	return res, nil
}

// Remove will remove one or more containers
// Besides a possible locking error, it might return hard filesystem errors
// Any other failure (ErrInvalidArgument, ErrNotFound) is a soft error that will be added the `warns`
func (vs *volumeStore) Remove(names []string) ([]string, []error, error) {
	var removed []string
	var warns []error
	var err error

	fn := func() error {
		for _, name := range names {
			if err := validateName(name); err != nil {
				warns = append(warns, fmt.Errorf("malformed volume name: %w (%w)", err, errdefs.ErrInvalidArgument))
				continue
			}

			// Remove legacy but ONLY if it validates the legacy syntax
			if validateLegacyName(name) == nil {
				dir := filepath.Join(vs.dir, name)
				if _, err := os.Stat(dir); err == nil {
					_ = os.RemoveAll(dir)
				}
			}

			dir := filepath.Join(vs.dir, strings.Replace(digest.FromString(name).String(), ":", "_", -1))
			// Does not exist, soft error
			if _, err := os.Lstat(dir); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					warns = append(warns, fmt.Errorf("no such volume: %s (%w)", name, errdefs.ErrNotFound))
					continue
				}
				return fmt.Errorf("filesystem error while trying to remove volumes from the volume store: %w", err)
			}
			// Hard filesystem error, hard error, and stop here
			if err := os.RemoveAll(dir); err != nil {
				return fmt.Errorf("filesystem error while trying to remove volumes from the volume store: %w", err)
			}
			// Otherwise, add it the list of successfully removed
			removed = append(removed, name)
		}
		return nil
	}

	if vs.locked == nil {
		err = lockutil.WithDirLock(vs.dir, fn)
	} else {
		err = fn()
	}

	return removed, warns, err
}

// Private helpers
func volumeSize(volume *native.Volume) (int64, error) {
	var size int64
	var walkFn = func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	}
	var err = filepath.Walk(volume.Mountpoint, walkFn)
	if err != nil {
		return 0, err
	}
	return size, nil
}
