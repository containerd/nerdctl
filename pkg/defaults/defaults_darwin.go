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

// This is a dummy file to allow usage of library functions
// on Darwin-based systems.
// Most functions and variables are stubs/no-ops

package defaults

import (
	"fmt"
	"os"
	"path/filepath"

	gocni "github.com/containerd/go-cni"
)

const (
	AppArmorProfileName = ""
	SeccompProfileName  = ""
	Runtime             = ""
)

func CNIPath() string {
	return gocni.DefaultCNIDir
}

func CNIRuntimeDir() (string, error) {
	if os.Geteuid() != 0 {
		return filepath.Join(xdgRuntimeDir(), "cni"), nil
	}
	return "/var/run/cni", nil
}

func CNINetConfPath() string {
	if os.Geteuid() != 0 {
		return filepath.Join(xdgConfigHome(), "cni", "net.d")
	}
	return gocni.DefaultNetDir
}

func DataRoot() string {
	if os.Geteuid() == 0 {
		return "/var/lib/nerdctl"
	}
	return filepath.Join(xdgDataHome(), "nerdctl")
}

func CgroupManager() string {
	return ""
}

func CgroupnsMode() string {
	return ""
}

func NerdctlTOML() string {
	if os.Geteuid() != 0 {
		return filepath.Join(xdgConfigHome(), "nerdctl", "nerdctl.toml")
	}
	return "/etc/nerdctl/nerdctl.toml"
}

func HostsDirs() []string {
	if os.Geteuid() != 0 {
		xch := xdgConfigHome()
		return []string{
			filepath.Join(xch, "containerd", "certs.d"),
			filepath.Join(xch, "docker", "certs.d"),
		}
	}
	return []string{"/etc/containerd/certs.d", "/etc/docker/certs.d"}
}

func HostGatewayIP() string {
	return ""
}

func CDISpecDirs() []string {
	if os.Geteuid() != 0 {
		return []string{
			filepath.Join(xdgConfigHome(), "cdi"),
			filepath.Join(xdgRuntimeDir(), "cdi"),
		}
	}
	return []string{"/etc/cdi", "/var/run/cdi"}
}

func xdgConfigHome() string {
	if xch := os.Getenv("XDG_CONFIG_HOME"); xch != "" {
		return xch
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".config")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".config")
	}
	return "/etc"
}

func xdgDataHome() string {
	if xdh := os.Getenv("XDG_DATA_HOME"); xdh != "" {
		return xdh
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".local", "share")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".local", "share")
	}
	return "/var/lib"
}

func xdgRuntimeDir() string {
	if xdr := os.Getenv("XDG_RUNTIME_DIR"); xdr != "" {
		return xdr
	}
	return fmt.Sprintf("/run/user/%d", os.Geteuid())
}
