/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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

// Package hoststore provides the interface for /var/lib/nerdctl/<ADDRHASH>/etchosts .
// Prioritize simplicity over scalability.
package hostsstore

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/AkihiroSuda/nerdctl/pkg/lockutil"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/go-cni"
)

const (
	// hostsDirBasename is the base name of /var/lib/nerdctl/<ADDRHASH>/etchosts
	hostsDirBasename = "etchosts"
	// metaJSON is stored as /var/lib/nerdctl/<ADDRHASH>/etchosts/<NS>/<ID>/meta.json
	metaJSON = "meta.json"
)

// HostsPath returns "/var/lib/nerdctl/<ADDRHASH>/etchosts/<NS>/<ID>/hosts"
func HostsPath(dataStore, ns, id string) string {
	if dataStore == "" || ns == "" || id == "" {
		panic(errdefs.ErrInvalidArgument)
	}
	return filepath.Join(dataStore, hostsDirBasename, ns, id, "hosts")
}

// ensureFile ensures a file with permission 0644.
// The file is initialized with no content.
// The dir (if not exists) is created with permission 0700.
func ensureFile(path string) error {
	if path == "" {
		return errdefs.ErrInvalidArgument
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE, 0644)
	if err != nil {
		f.Close()
	}
	return err
}

// EnsureHostsFile is used for creating mount-bindable /etc/hosts file.
// The file is initialized with no content.
func EnsureHostsFile(dataStore, ns, id string) (string, error) {
	lockDir := filepath.Join(dataStore, hostsDirBasename)
	if err := os.MkdirAll(lockDir, 0700); err != nil {
		return "", err
	}
	path := HostsPath(dataStore, ns, id)
	fn := func() error {
		return ensureFile(path)
	}
	err := lockutil.WithDirLock(lockDir, fn)
	return path, err
}

func NewStore(dataStore string) (Store, error) {
	store := &store{
		dataStore: dataStore,
		hostsD:    filepath.Join(dataStore, hostsDirBasename),
	}
	return store, os.MkdirAll(store.hostsD, 0700)
}

type Meta struct {
	Namespace string
	ID        string
	Networks  map[string]*cni.CNIResult
	Hostname  string
	Name      string
}

type Store interface {
	Acquire(Meta) error
	Release(ns, id string) error
}

type store struct {
	// dataStore is /var/lib/nerdctl/<ADDRHASH>
	dataStore string
	// hostsD is /var/lib/nerdctl/<ADDRHASH>/etchosts
	hostsD string
}

func (x *store) Acquire(meta Meta) error {
	fn := func() error {
		hostsPath := HostsPath(x.dataStore, meta.Namespace, meta.ID)
		if err := ensureFile(hostsPath); err != nil {
			return err
		}
		metaB, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		metaPath := filepath.Join(x.hostsD, meta.Namespace, meta.ID, metaJSON)
		if err := ioutil.WriteFile(metaPath, metaB, 0644); err != nil {
			return err
		}
		return newUpdater(x.hostsD).update()
	}
	return lockutil.WithDirLock(x.hostsD, fn)
}

func (x *store) Release(ns, id string) error {
	fn := func() error {
		d := filepath.Join(x.hostsD, ns, id)
		if _, err := os.Stat(d); errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err := os.RemoveAll(d); err != nil {
			return err
		}
		return newUpdater(x.hostsD).update()
	}
	return lockutil.WithDirLock(x.hostsD, fn)
}
