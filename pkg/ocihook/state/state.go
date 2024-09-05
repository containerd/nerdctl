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

// Package state provides a store to retrieve and save container lifecycle related information
// This is typically used by oci-hooks for information that cannot be retrieved / updated otherwise
// Specifically, the state carries container start time, and transient information about possible failures during
// hook events processing.
// All store methods are safe to use concurrently and only write atomically.
// Since the state is transient and carrying solely informative data, errors returned from here could be treated as
// soft-failures.
// Note that locking is done at the container state directory level.
// state is currently used by ocihooks and for read by dockercompat (to display started-at time)
package state

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/containerd/nerdctl/v2/pkg/store"
)

// lifecycleFile is the name of file carrying the container information, relative to stateDir
const lifecycleFile = "lifecycle.json"

// ErrLifecycleStore will wrap all errors here
var ErrLifecycleStore = errors.New("lifecycle-store error")

// New will return a lifecycle struct for the container which stateDir is passed as argument
func New(stateDir string) (*Store, error) {
	st, err := store.New(stateDir, 0, 0)
	if err != nil {
		return nil, errors.Join(ErrLifecycleStore, err)
	}

	return &Store{
		safeStore: st,
	}, nil
}

// Store exposes methods to retrieve and transform state information about containers.
type Store struct {
	safeStore store.Store

	// StartedAt reflects the time at which we received the oci-hook onCreateRuntime event
	StartedAt   time.Time `json:"started_at"`
	CreateError bool      `json:"create_error"`
}

// Load will populate the struct with existing in-store lifecycle information
func (lf *Store) Load() (err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrLifecycleStore, err)
		}
	}()

	return lf.safeStore.WithLock(lf.rawLoad)
}

// Transform should be used to perform random mutations
func (lf *Store) Transform(fun func(lf *Store) error) (err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrLifecycleStore, err)
		}
	}()

	return lf.safeStore.WithLock(func() error {
		err = lf.rawLoad()
		if err != nil {
			return err
		}
		err = fun(lf)
		if err != nil {
			return err
		}
		return lf.rawSave()
	})
}

// Delete will destroy the lifecycle data
func (lf *Store) Delete() (err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrLifecycleStore, err)
		}
	}()

	return lf.safeStore.WithLock(lf.rawDelete)
}

func (lf *Store) rawLoad() (err error) {
	data, err := lf.safeStore.Get(lifecycleFile)
	if err == nil {
		err = json.Unmarshal(data, lf)
	} else if errors.Is(err, store.ErrNotFound) {
		err = nil
	}

	return err
}

func (lf *Store) rawSave() (err error) {
	data, err := json.Marshal(lf)
	if err != nil {
		return err
	}
	return lf.safeStore.Set(data, lifecycleFile)
}

func (lf *Store) rawDelete() (err error) {
	return lf.safeStore.Delete(lifecycleFile)
}
