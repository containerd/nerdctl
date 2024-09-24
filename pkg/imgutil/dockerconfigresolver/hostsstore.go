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

package dockerconfigresolver

import (
	"errors"
	"os"

	"github.com/containerd/containerd/v2/core/remotes/docker/config"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"
)

// validateDirectories inspect a slice of strings and returns the ones that are valid readable directories
func validateDirectories(orig []string) []string {
	ss := []string{}
	for _, v := range orig {
		fi, err := os.Stat(v)
		if err != nil || !fi.IsDir() {
			if !errors.Is(err, os.ErrNotExist) {
				log.L.WithError(err).Warnf("Ignoring hosts location %q", v)
			}
			continue
		}
		ss = append(ss, v)
	}
	return ss
}

// hostDirsFromRoot will retrieve a host.toml file for the namespace host, possibly trying without port
// if the requested port is standard.
// https://github.com/containerd/nerdctl/issues/3047
func hostDirsFromRoot(registryURL *RegistryURL, dirs []string) (string, error) {
	hostsDirs := validateDirectories(dirs)

	// Go through the configured system location to consider for hosts.toml files
	for _, hostsDir := range hostsDirs {
		found, err := config.HostDirFromRoot(hostsDir)(registryURL.Host)
		// If we errored with anything but NotFound, or if we found one, return now
		if (err != nil && !errdefs.IsNotFound(err)) || (found != "") {
			return found, err
		}
		// If not found, and the port is standard, try again without the port
		if registryURL.Port() == standardHTTPSPort {
			found, err = config.HostDirFromRoot(hostsDir)(registryURL.Hostname())
			if (err != nil && !errors.Is(err, errdefs.ErrNotFound)) || (found != "") {
				return found, err
			}
		}
	}
	return "", nil
}
