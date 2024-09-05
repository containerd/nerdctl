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

package hostsstore

import (
	"github.com/containerd/nerdctl/v2/pkg/netutil"
)

// createLine returns a line string slice.
// line is like "foo foo.nw0 bar bar.nw0\n"
// for `nerdctl --name=foo --hostname=bar --network=n0`.
//
// May return an empty string slice
func createLine(thatNetwork string, meta *Meta, myNetworks map[string]struct{}) []string {
	line := []string{}
	if _, ok := myNetworks[thatNetwork]; !ok {
		// Do not add lines for other networks
		return line
	}
	baseHostnames := []string{meta.Hostname}
	if meta.Name != "" {
		baseHostnames = append(baseHostnames, meta.Name)
	}

	for _, baseHostname := range baseHostnames {
		line = append(line, baseHostname)
		if thatNetwork != netutil.DefaultNetworkName {
			// Do not add a entry like "foo.bridge"
			line = append(line, baseHostname+"."+thatNetwork)
		}
	}
	return line
}
