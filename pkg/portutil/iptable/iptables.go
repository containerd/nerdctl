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
	"regexp"
	"strconv"
	"strings"
)

type PortRule struct {
	IP   string
	Port uint64
}

// ParseIPTableRules takes a slice of iptables rules as input and returns a
// slice of PortRule containing the parsed destination IP and port from the
// rules. When a rule has no -d flag, IP is empty (meaning the rule applies to
// all addresses).
func ParseIPTableRules(rules []string) []PortRule {
	portRules := []PortRule{}

	dportsRegex := regexp.MustCompile(`--dports ((,?\d+)+)`)
	dportRegex := regexp.MustCompile(`--dport (\d+)`)
	destRegex := regexp.MustCompile(`-d (\S+?)(?:/\d+)?\s`)

	for _, rule := range rules {
		var ports []string

		if matches := dportsRegex.FindStringSubmatch(rule); len(matches) > 1 {
			ports = strings.Split(matches[1], ",")
		} else if matches := dportRegex.FindStringSubmatch(rule); len(matches) > 1 {
			ports = []string{matches[1]}
		} else {
			continue
		}

		var ip string
		if destMatches := destRegex.FindStringSubmatch(rule); len(destMatches) > 1 {
			ip = destMatches[1]
		}

		for _, portStr := range ports {
			port64, err := strconv.ParseUint(portStr, 10, 16)
			if err != nil {
				continue
			}
			portRules = append(portRules, PortRule{IP: ip, Port: port64})
		}
	}

	return portRules
}
