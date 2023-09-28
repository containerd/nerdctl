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

// ParseIPTableRules takes a slice of iptables rules as input and returns a slice of
// uint64 containing the parsed destination port numbers from the rules.
func ParseIPTableRules(rules []string) []uint64 {
	ports := []uint64{}

	// Regex to match the '--dports' option followed by the port number
	dportRegex := regexp.MustCompile(`--dports ((,?\d+)+)`)

	for _, rule := range rules {
		matches := dportRegex.FindStringSubmatch(rule)
		if len(matches) > 1 {
			for _, _match := range strings.Split(matches[1], ",") {
				port64, err := strconv.ParseUint(_match, 10, 16)
				if err != nil {
					continue
				}
				ports = append(ports, port64)
			}
		}
	}

	return ports
}
