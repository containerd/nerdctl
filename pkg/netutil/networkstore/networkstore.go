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

package networkstore

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
	networkConfigName     = "network-config.json"
)

var ErrNetworkStore = errors.New("network-store error")

func New(dataStore, namespace, containerID string) (ns *NetworkStore, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrNetworkStore, err)
		}
	}()

	if dataStore == "" || namespace == "" || containerID == "" {
		return nil, fmt.Errorf("either dataStore or namespace or containerID is empty")
	}

	st, err := store.New(filepath.Join(dataStore, containersDirBaseName, namespace, containerID), 0, 0o600)
	if err != nil {
		return nil, err
	}

	return &NetworkStore{
		safeStore: st,
	}, nil
}

type NetworkConfig struct {
	PortMappings []cni.PortMapping `json:"portMappings,omitempty"`
}

type NetworkStore struct {
	safeStore store.Store

	NetConf NetworkConfig
}

func (ns *NetworkStore) Acquire(netConf NetworkConfig) (err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrNetworkStore, err)
		}
	}()

	netConfJSON, err := json.Marshal(netConf)
	if err != nil {
		return fmt.Errorf("failed to marshal network config to JSON: %w", err)
	}

	return ns.safeStore.WithLock(func() error {
		return ns.safeStore.Set(netConfJSON, networkConfigName)
	})
}

func (ns *NetworkStore) Load() (err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrNetworkStore, err)
		}
	}()

	return ns.safeStore.WithLock(func() error {
		doesExist, err := ns.safeStore.Exists(networkConfigName)
		if err != nil || !doesExist {
			return err
		}

		data, err := ns.safeStore.Get(networkConfigName)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				err = nil
			}
			return err
		}

		var netConf NetworkConfig
		if err := json.Unmarshal(data, &netConf); err != nil {
			return fmt.Errorf("failed to parse network config %v: %w", netConf, err)
		}
		ns.NetConf = netConf

		return err
	})
}
