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

package netutil

import (
	"os"
	"path/filepath"

	"github.com/containernetworking/cni/libcni"

	"github.com/containerd/errdefs"

	"github.com/containerd/nerdctl/v2/pkg/internal/filesystem"
)

// NOTE: libcni is not safe to use concurrently - or at least delegates concurrency management to the consumer.
// Furthermore, CNIEnv (prior to this) is assuming the filesystem is ACID and other TOCTOU faults.
// This small set of methods here are meant to isolate CNIEnv entirely from the filesystem.
// This is NOT proper - we should instead use the Store implementation, which is the generic abstraction for ACID
// operations - but for now that will do, waiting for a full rewrite of CNIEnv.

func fsEnsureRoot(e *CNIEnv, namespace string) error {
	path := e.NetconfPath
	if namespace != "" {
		path = filepath.Join(e.NetconfPath, namespace)
	}
	return os.MkdirAll(path, 0755)
}

func fsRemove(e *CNIEnv, net *NetworkConfig) error {
	fn := func() error {
		if err := os.RemoveAll(net.File); err != nil {
			return err
		}
		return net.clean()
	}
	return filesystem.WithLock(filepath.Join(e.NetconfPath, ".nerdctl.lock"), fn)
}

func fsExists(e *CNIEnv, name string) (bool, error) {
	fi, err := os.Stat(getConfigPathForNetworkName(e, name))
	return !os.IsNotExist(err) && !fi.IsDir(), err
}

func fsWrite(e *CNIEnv, net *NetworkConfig) error {
	filename := getConfigPathForNetworkName(e, net.Name)
	// FIXME: note that this is still problematic.
	// Concurrent access may independently first figure out that a given network is missing, and while the lock
	// here will prevent concurrent writes, one of the routines will fail.
	// Consuming code MUST account for that scenario.
	return filesystem.WithLock(filepath.Join(e.NetconfPath, ".nerdctl.lock"), func() error {
		if _, err := os.Stat(filename); err == nil {
			return errdefs.ErrAlreadyExists
		}
		return os.WriteFile(filename, net.Bytes, 0644)
	})
}

func fsRead(e *CNIEnv) ([]*NetworkConfig, error) {
	var nc []*NetworkConfig
	var err error
	err = filesystem.WithReadOnlyLock(filepath.Join(e.NetconfPath, ".nerdctl.lock"), func() error {
		namespaced := []string{}
		var common []string
		common, err = libcni.ConfFiles(e.NetconfPath, []string{".conf", ".conflist", ".json"})
		if err != nil {
			return err
		}
		if e.Namespace != "" {
			namespaced, err = libcni.ConfFiles(filepath.Join(e.NetconfPath, e.Namespace), []string{".conf", ".conflist", ".json"})
			if err != nil {
				return err
			}
		}
		nc, err = cniLoad(append(common, namespaced...))
		return err
	})
	return nc, err
}

func getConfigPathForNetworkName(e *CNIEnv, netName string) string {
	if netName == DefaultNetworkName || e.Namespace == "" {
		return filepath.Join(e.NetconfPath, "nerdctl-"+netName+".conflist")
	}
	return filepath.Join(e.NetconfPath, e.Namespace, "nerdctl-"+netName+".conflist")
}
