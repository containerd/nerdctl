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
	gocni "github.com/containerd/go-cni"
)

const (
	AppArmorProfileName = ""
	SeccompProfileName  = ""
	Runtime             = "wtf.sbk.runj.v1"
)

func DataRoot() string {
	return "/var/lib/nerdctl"
}

func CNIPath() string {
	// default: /opt/cni/bin
	return gocni.DefaultCNIDir
}

func CNINetConfPath() string {
	return gocni.DefaultNetDir
}

func CNIRuntimeDir() string {
	return "/run/cni"
}

func BuildKitHost() string {
	return "unix:///run/buildkit/buildkitd.sock"
}

func CgroupManager() string {
	return ""
}

func CgroupnsMode() string {
	return ""
}

func NerdctlTOML() string {
	return "/etc/nerdctl/nerdctl.toml"
}

func HostsDirs() []string {
	return []string{"/etc/containerd/certs.d", "/etc/docker/certs.d"}
}

func HostGatewayIP() string {
	return ""
}
