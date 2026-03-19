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
	"strings"

	"github.com/coreos/go-iptables/iptables"
)

// Chain used for port forwarding rules: https://www.cni.dev/plugins/current/meta/portmap/#dnat
const cniDnatChain = "CNI-HOSTPORT-DNAT"

// cniDNChainPrefix is the prefix for per-container DNAT sub-chains created by
// the CNI portmap plugin. These sub-chains contain the actual DNAT rules with
// destination IP filtering (e.g. -d 192.168.1.141/32 --dport 80 -j DNAT).
const cniDNChainPrefix = "CNI-DN-"

func ReadIPTables(table string) ([]string, error) {
	ipt, err := iptables.New()
	if err != nil {
		return nil, err
	}

	chainExists, _ := ipt.ChainExists(table, cniDnatChain)
	if !chainExists {
		return nil, nil
	}

	parentRules, err := ipt.List(table, cniDnatChain)
	if err != nil {
		return nil, err
	}

	// Read per-container DNAT sub-chains (CNI-DN-*) which contain the actual
	// DNAT rules with both destination IP and port information.
	// The parent chain only dispatches by port and does not include destination IP.
	var rules []string
	for _, rule := range parentRules {
		fields := strings.Fields(rule)
		for i, f := range fields {
			if f == "-j" && i+1 < len(fields) && strings.HasPrefix(fields[i+1], cniDNChainPrefix) {
				subRules, err := ipt.List(table, fields[i+1])
				if err != nil {
					break
				}
				rules = append(rules, subRules...)
				break
			}
		}
	}

	// Fall back to parent chain rules if no sub-chain rules were found.
	if len(rules) == 0 {
		rules = parentRules
	}

	return rules, nil
}
