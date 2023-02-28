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
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/containerd/containerd/plugin"
	gocni "github.com/containerd/go-cni"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/sirupsen/logrus"
)

const AppArmorProfileName = "nerdctl-default"
const Runtime = plugin.RuntimeRuncV2

func DataRoot() string {
	if !rootlessutil.IsRootless() {
		return "/var/lib/nerdctl"
	}
	xdh, err := rootlessutil.XDGDataHome()
	if err != nil {
		panic(err)
	}
	return filepath.Join(xdh, "nerdctl")
}

func CNIPath() string {
	candidates := []string{
		"/usr/local/libexec/cni",
		"/usr/local/lib/cni",
		"/usr/libexec/cni", // Fedora
		"/usr/lib/cni",     // debian (containernetworking-plugins)
	}
	if rootlessutil.IsRootless() {
		home := os.Getenv("HOME")
		if home == "" {
			panic("environment variable HOME is not set")
		}
		candidates = append([]string{
			// NOTE: These user paths are not defined in XDG
			filepath.Join(home, ".local/libexec/cni"),
			filepath.Join(home, ".local/lib/cni"),
			filepath.Join(home, "opt/cni/bin"),
		}, candidates...)
	}

	for _, f := range candidates {
		if _, err := os.Stat(f); err == nil {
			return f
		}
	}

	// default: /opt/cni/bin
	return gocni.DefaultCNIDir
}

func CNINetConfPath() string {
	if !rootlessutil.IsRootless() {
		return gocni.DefaultNetDir
	}
	xch, err := rootlessutil.XDGConfigHome()
	if err != nil {
		panic(err)
	}
	return filepath.Join(xch, "cni/net.d")
}

func CNIRuntimeDir() string {
	if !rootlessutil.IsRootless() {
		return "/run/cni"
	}
	xdr, err := rootlessutil.XDGRuntimeDir()
	if err != nil {
		logrus.Warn(err)
		xdr = fmt.Sprintf("/run/user/%d", rootlessutil.ParentEUID())
	}
	return fmt.Sprintf("%s/cni", xdr)
}

func BuildKitHost() string {
	if !rootlessutil.IsRootless() {
		return "unix:///run/buildkit/buildkitd.sock"
	}
	xdr, err := rootlessutil.XDGRuntimeDir()
	if err != nil {
		logrus.Warn(err)
		xdr = fmt.Sprintf("/run/user/%d", rootlessutil.ParentEUID())
	}
	return fmt.Sprintf("unix://%s/buildkit/buildkitd.sock", xdr)
}

func NerdctlTOML() string {
	if !rootlessutil.IsRootless() {
		return "/etc/nerdctl/nerdctl.toml"
	}
	xch, err := rootlessutil.XDGConfigHome()
	if err != nil {
		panic(err)
	}
	return filepath.Join(xch, "nerdctl/nerdctl.toml")
}

func HostsDirs() []string {
	if !rootlessutil.IsRootless() {
		return []string{"/etc/containerd/certs.d", "/etc/docker/certs.d"}
	}
	xch, err := rootlessutil.XDGConfigHome()
	if err != nil {
		panic(err)
	}
	return []string{
		filepath.Join(xch, "containerd/certs.d"),
		filepath.Join(xch, "docker/certs.d"),
	}
}

// HostGatewayIP returns the non-loop-back host ip if available and returns empty string if running into error.
func HostGatewayIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}
