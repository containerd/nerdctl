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

// Package hostsstore provides the interface for /var/lib/nerdctl/<ADDRHASH>/etchosts
// Prioritizes simplicity over scalability.
// All methods perform atomic writes and are safe to use concurrently.
// Note that locking is done per namespace.
// hostsstore is currently by container rename, remove, network managers, and ocihooks
// Finally, NOTE:
// Since we will write to the hosts file after it is mounted in the container, we cannot use our atomic write method
// as the inode would change on rename.
// Henceforth, hosts file mutation uses filesystem methods instead, making it the one exception that has to bypass
// the Store implementation.
package hostsstore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	types100 "github.com/containernetworking/cni/pkg/types/100"

	"github.com/containerd/errdefs"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/store"
)

const (
	// hostsDirBasename is the base name of /var/lib/nerdctl/<ADDRHASH>/etchosts
	hostsDirBasename = "etchosts"
	// metaJSON is stored as hostsDirBasename/<NS>/<ID>/meta.json
	metaJSON = "meta.json"
	// hostsFile is stored as hostsDirBasename/<NS>/<ID>/hosts
	hostsFile = "hosts"
)

// ErrHostsStore will wrap all errors here
var ErrHostsStore = errors.New("hosts-store error")

func New(dataStore string, namespace string) (retStore Store, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrHostsStore, err)
		}
	}()

	if dataStore == "" || namespace == "" {
		return nil, store.ErrInvalidArgument
	}

	st, err := store.New(filepath.Join(dataStore, hostsDirBasename, namespace), 0, 0o644)
	if err != nil {
		return nil, err
	}

	return &hostsStore{
		safeStore: st,
	}, nil
}

type Meta struct {
	ID         string
	Networks   map[string]*types100.Result
	Hostname   string
	ExtraHosts map[string]string // host:ip
	Name       string
}

type Store interface {
	Acquire(Meta) error
	Release(id string) error
	Update(id, newName string) error
	HostsPath(id string) (location string, err error)
	Delete(id string) (err error)
	AllocHostsFile(id string, content []byte) (location string, err error)
}

type hostsStore struct {
	safeStore store.Store
}

func (x *hostsStore) Acquire(meta Meta) (err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrHostsStore, err)
		}
	}()

	return x.safeStore.WithLock(func() error {
		var loc string
		loc, err = x.safeStore.Location(meta.ID, hostsFile)
		if err != nil {
			return err
		}

		if err = os.WriteFile(loc, []byte{}, 0o644); err != nil {
			return errors.Join(store.ErrSystemFailure, err)
		}

		var content []byte
		content, err = json.Marshal(meta)
		if err != nil {
			return err
		}

		if err = x.safeStore.Set(content, meta.ID, metaJSON); err != nil {
			return err
		}

		return x.updateAllHosts()
	})
}

// Release is triggered by Poststop hooks.
// It is called after the containerd task is deleted but before the delete operation returns.
func (x *hostsStore) Release(id string) (err error) {
	// We remove "meta.json" but we still retain the "hosts" file
	// because it is needed for restarting. The "hosts" is removed on
	// `nerdctl rm`.
	// https://github.com/rootless-containers/rootlesskit/issues/220#issuecomment-783224610
	defer func() {
		if err != nil {
			err = errors.Join(ErrHostsStore, err)
		}
	}()

	return x.safeStore.WithLock(func() error {
		if err = x.safeStore.Delete(id, metaJSON); err != nil {
			return err
		}

		return x.updateAllHosts()
	})
}

// AllocHostsFile is used for creating mount-bindable /etc/hosts file.
func (x *hostsStore) AllocHostsFile(id string, content []byte) (location string, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrHostsStore, err)
		}
	}()

	err = x.safeStore.WithLock(func() error {
		err = x.safeStore.GroupEnsure(id)
		if err != nil {
			return err
		}

		var loc string
		loc, err = x.safeStore.Location(id, hostsFile)
		if err != nil {
			return err
		}

		err = os.WriteFile(loc, content, 0o644)
		if err != nil {
			err = errors.Join(store.ErrSystemFailure, err)
		}

		return err
	})
	if err != nil {
		return "", err
	}

	return x.safeStore.Location(id, hostsFile)
}

func (x *hostsStore) Delete(id string) (err error) {
	err = x.safeStore.WithLock(func() error { return x.safeStore.Delete(id) })
	if err != nil {
		err = errors.Join(ErrHostsStore, err)
	}

	return err
}

func (x *hostsStore) HostsPath(id string) (location string, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrHostsStore, err)
		}
	}()

	return x.safeStore.Location(id, hostsFile)
}

func (x *hostsStore) Update(id, newName string) (err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrHostsStore, err)
		}
	}()

	return x.safeStore.WithLock(func() error {
		var content []byte
		if content, err = x.safeStore.Get(id, metaJSON); err != nil {
			return err
		}

		meta := &Meta{}
		if err = json.Unmarshal(content, meta); err != nil {
			return err
		}

		meta.Name = newName
		content, err = json.Marshal(meta)
		if err != nil {
			return err
		}

		if err = x.safeStore.Set(content, id, metaJSON); err != nil {
			return err
		}

		return x.updateAllHosts()
	})
}

func (x *hostsStore) updateAllHosts() (err error) {
	entries, err := x.safeStore.List()
	if err != nil {
		return err
	}

	metasByEntry := map[string]*Meta{}
	metasByIP := map[string]*Meta{}
	networkNameByIP := map[string]string{}

	// Phase 1: read all meta files
	for _, entry := range entries {
		var content []byte
		content, err = x.safeStore.Get(entry, metaJSON)
		if err != nil {
			log.L.WithError(err).Debugf("unable to read %q", entry)
			continue
		}
		meta := &Meta{}
		if err = json.Unmarshal(content, meta); err != nil {
			log.L.WithError(err).Warnf("unable to unmarshell %q", entry)
			continue
		}
		metasByEntry[entry] = meta

		for netName, cniRes := range meta.Networks {
			for _, ipCfg := range cniRes.IPs {
				if ip := ipCfg.Address.IP; ip != nil {
					if ip.IsLoopback() || ip.IsUnspecified() {
						continue
					}
					ipStr := ip.String()
					metasByIP[ipStr] = meta
					networkNameByIP[ipStr] = netName
				}
			}
		}
	}

	// Phase 2: write hosts files
	for _, entry := range entries {
		myMeta, ok := metasByEntry[entry]
		if !ok {
			log.L.WithError(errdefs.ErrNotFound).Debugf("hostsstore metadata %q not found in %q?", metaJSON, entry)
			continue
		}

		myNetworks := make(map[string]struct{})
		for nwName := range myMeta.Networks {
			myNetworks[nwName] = struct{}{}
		}

		var content []byte
		content, err = x.safeStore.Get(entry, hostsFile)
		if err != nil {
			log.L.WithError(err).Errorf("unable to retrieve the hosts file for %q", entry)
			continue
		}

		// parse the hosts file, keep the original host record
		// retain custom /etc/hosts entries outside <nerdctl> </nerdctl> region
		var buf bytes.Buffer
		if content != nil {
			if err = parseHostsButSkipMarkedRegion(&buf, bytes.NewReader(content)); err != nil {
				log.L.WithError(err).Errorf("failed to read hosts file for %q", entry)
				continue
			}
		}

		buf.WriteString(fmt.Sprintf("# %s\n", MarkerBegin))
		buf.WriteString("127.0.0.1	localhost localhost.localdomain\n")
		buf.WriteString("::1		localhost localhost.localdomain\n")

		// keep extra hosts first
		for host, ip := range myMeta.ExtraHosts {
			buf.WriteString(fmt.Sprintf("%-15s %s\n", ip, host))
		}

		for ip, netName := range networkNameByIP {
			meta := metasByIP[ip]
			if line := createLine(netName, meta, myNetworks); len(line) != 0 {
				buf.WriteString(fmt.Sprintf("%-15s %s\n", ip, strings.Join(line, " ")))
			}
		}

		buf.WriteString(fmt.Sprintf("# %s\n", MarkerEnd))

		var loc string
		loc, err = x.safeStore.Location(entry, hostsFile)
		if err != nil {
			return err
		}

		err = os.WriteFile(loc, buf.Bytes(), 0o644)
		if err != nil {
			log.L.WithError(err).Errorf("failed to write hosts file for %q", entry)
		}
	}
	return nil
}
