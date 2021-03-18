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

package defaults

import (
	"fmt"
	"os"
	"path/filepath"

	gocni "github.com/containerd/go-cni"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/sirupsen/logrus"
)

const AppArmorProfileName = "nerdctl-default"

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
		"/usr/libexec/cni", // Fedora
	}
	if rootlessutil.IsRootless() {
		home := os.Getenv("HOME")
		if home == "" {
			panic("environment variable HOME is not set")
		}
		candidates = append([]string{
			// NOTE: These user paths are not defined in XDG
			filepath.Join(home, ".local/libexec/cni"),
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
