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

package iptable

import (
	"github.com/coreos/go-iptables/iptables"
)

// Chain used for port forwarding rules: https://www.cni.dev/plugins/current/meta/portmap/#dnat
const cniDnatChain = "CNI-HOSTPORT-DNAT"

func ReadIPTables(table string) ([]string, error) {
	ipt, err := iptables.New()
	if err != nil {
		return nil, err
	}

	var rules []string
	chainExists, _ := ipt.ChainExists(table, cniDnatChain)
	if chainExists {
		rules, err = ipt.List(table, cniDnatChain)
		if err != nil {
			return nil, err
		}
	}

	return rules, nil
}
