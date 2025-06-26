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

package portstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/containerd/go-cni"

	"github.com/containerd/nerdctl/v2/pkg/store"
)

const (
	containersDirBaseName = "containers"
	portsFileName         = "ports.json"
)

var ErrPortStore = errors.New("port-store error")

func New(dataStore, namespace, containerID string) (ns *PortStore, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrPortStore, err)
		}
	}()

	if dataStore == "" || namespace == "" || containerID == "" {
		return nil, fmt.Errorf("either dataStore or namespace or containerID is empty")
	}

	st, err := store.New(filepath.Join(dataStore, containersDirBaseName, namespace, containerID), 0, 0o600)
	if err != nil {
		return nil, err
	}

	return &PortStore{
		safeStore: st,
	}, nil
}

type PortStore struct {
	safeStore store.Store

	PortMappings []cni.PortMapping
}

func (ps *PortStore) Acquire(portMappings []cni.PortMapping) (err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrPortStore, err)
		}
	}()

	portsJSON, err := json.Marshal(portMappings)
	if err != nil {
		return fmt.Errorf("failed to marshal port mappings to JSON: %w", err)
	}

	return ps.safeStore.WithLock(func() error {
		return ps.safeStore.Set(portsJSON, portsFileName)
	})
}

func (ps *PortStore) Load() (err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrPortStore, err)
		}
	}()

	return ps.safeStore.WithLock(func() error {
		doesExist, err := ps.safeStore.Exists(portsFileName)
		if err != nil || !doesExist {
			return err
		}

		data, err := ps.safeStore.Get(portsFileName)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				err = nil
			}
			return err
		}

		var ports []cni.PortMapping
		if err := json.Unmarshal(data, &ports); err != nil {
			return fmt.Errorf("failed to parse port mappings %v: %w", ports, err)
		}
		ps.PortMappings = ports

		return err
	})
}
