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

package defaults

import (
	"os"

	"github.com/containerd/cgroups"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
)

func IsSystemdAvailable() bool {
	fi, err := os.Lstat("/run/systemd/system")
	if err != nil {
		return false
	}
	return fi.IsDir()
}

// CgroupManager defaults to:
// - "systemd"  on v2 (rootful & rootless)
// - "cgroupfs" on v1 rootful
// - "none"     on v1 rootless
func CgroupManager() string {
	if cgroups.Mode() == cgroups.Unified && IsSystemdAvailable() {
		return "systemd"
	}
	if rootlessutil.IsRootless() {
		return "none"
	}
	return "cgroupfs"
}

func CgroupnsMode() string {
	if cgroups.Mode() == cgroups.Unified {
		return "private"
	}
	return "host"
}

func CgroupsVersion() string {
	if cgroups.Mode() == cgroups.Unified {
		return "2"
	}

	return "1"
}
