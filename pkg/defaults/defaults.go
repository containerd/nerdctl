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
	"path/filepath"

	"github.com/AkihiroSuda/nerdctl/pkg/rootlessutil"
	gocni "github.com/containerd/go-cni"
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
		panic(err)
	}
	return fmt.Sprintf("unix://%s/buildkit/buildkitd.sock", xdr)
}
