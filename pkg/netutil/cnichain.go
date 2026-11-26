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
	"crypto/sha512"
	"fmt"
)

// FormatCNIChainName returns the iptables chain name that the CNI plugins
// (e.g., portmap) generate for the given network name and container ID.
//
// Equivalent to FormatChainName in github.com/containernetworking/plugins/pkg/utils,
// reimplemented here to avoid pulling in the dependencies of that package
// (netlink, go-iptables, knftables, ...).
//
// https://github.com/containernetworking/plugins/blob/v1.10.0/pkg/utils/utils.go#L27-L61
func FormatCNIChainName(name, id string) string {
	const (
		maxChainLength = 28
		chainPrefix    = "CNI-"
	)
	h := sha512.Sum512([]byte(name + id))
	return fmt.Sprintf("%s%x", chainPrefix, h)[:maxChainLength]
}
