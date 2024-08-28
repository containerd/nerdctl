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

// Package namestore provides a simple store for containers to exclusively acquire and release names.
// All methods are safe to use concurrently.
// Note that locking of the store is done at the namespace level.
// The namestore is currently used by container create, remove, rename, and as part of the ocihook events cycle.
package namestore

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/identifiers"
	"github.com/containerd/nerdctl/v2/pkg/store"
)

// ErrNameStore will wrap all errors here
var ErrNameStore = errors.New("name-store error")

// New will return a NameStore for a given namespace.
func New(stateDir, namespace string) (NameStore, error) {
	if namespace == "" {
		return nil, errors.Join(ErrNameStore, store.ErrInvalidArgument)
	}

	st, err := store.New(filepath.Join(stateDir, namespace), 0, 0)
	if err != nil {
		return nil, errors.Join(ErrNameStore, err)
	}

	return &nameStore{
		safeStore: st,
	}, nil
}

// NameStore allows acquiring, releasing and renaming.
// "names" must abide by identifiers.ValidateDockerCompat
// A container cannot release or rename a name it does not own.
// A container cannot acquire a name that is already owned by another container.
// Re-acquiring a name does not error and is a no-op.
// Double releasing a name will error.
// Note that technically a given container may acquire multiple different names, although this is not
// something we do in the codebase.
type NameStore interface {
	// Acquire exclusively grants `name` to container with `id`.
	Acquire(name, id string) error
	// Acquire allows the container owning a specific name to release it
	Release(name, id string) error
	// Rename allows the container owning a specific name to change it to newName (if available)
	Rename(oldName, id, newName string) error
}

type nameStore struct {
	safeStore store.Store
}

func (x *nameStore) Acquire(name, id string) (err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrNameStore, err)
		}
	}()

	if err = identifiers.ValidateDockerCompat(name); err != nil {
		return err
	}

	return x.safeStore.WithLock(func() error {
		var previousID []byte
		previousID, err = x.safeStore.Get(name)
		if err != nil {
			if !errors.Is(err, store.ErrNotFound) {
				return err
			}
		} else if string(previousID) == "" {
			// This has happened in the past, probably following some other error condition of OS restart
			// We do warn about it, but do not hard-error and let the new container acquire the name
			log.L.Warnf("name %q was locked by an empty id - this is abnormal and should be reported", name)
		} else if string(previousID) != id {
			// If the name is already used by another container, that is a hard error
			return fmt.Errorf("name %q is already used by ID %q", name, previousID)
		}

		// If the id was the same, we are "re-acquiring".
		// Maybe containerd was bounced, so previously running containers that would get restarted will go again through
		// onCreateRuntime (unlike in a "normal" stop/start flow), without ever had gone through onPostStop.
		// As such, reacquiring by the same id is not a bug...
		// See: https://github.com/containerd/nerdctl/issues/3354
		return x.safeStore.Set([]byte(id), name)
	})
}

func (x *nameStore) Release(name, id string) (err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrNameStore, err)
		}
	}()

	if err = identifiers.ValidateDockerCompat(name); err != nil {
		return err
	}

	return x.safeStore.WithLock(func() error {
		var content []byte
		content, err = x.safeStore.Get(name)
		if err != nil {
			return err
		}

		if string(content) != id {
			// Never seen this, but technically possible if downstream code is messed-up
			return fmt.Errorf("cannot release name %q (used by ID %q, not by %q)", name, content, id)
		}

		return x.safeStore.Delete(name)
	})
}

func (x *nameStore) Rename(oldName, id, newName string) (err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrNameStore, err)
		}
	}()

	if err = identifiers.ValidateDockerCompat(newName); err != nil {
		return err
	}

	return x.safeStore.WithLock(func() error {
		var doesExist bool
		var content []byte
		doesExist, err = x.safeStore.Exists(newName)
		if err != nil {
			return err
		}

		if doesExist {
			content, err = x.safeStore.Get(newName)
			if err != nil {
				return err
			}
			return fmt.Errorf("name %q is already used by ID %q", newName, string(content))
		}

		content, err = x.safeStore.Get(oldName)
		if err != nil {
			return err
		}

		if string(content) != id {
			return fmt.Errorf("name %q is used by ID %q, not by %q", oldName, content, id)
		}

		err = x.safeStore.Set(content, newName)
		if err != nil {
			return err
		}

		return x.safeStore.Delete(oldName)
	})
}
