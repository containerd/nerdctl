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

package iptables

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/coreos/go-iptables/iptables"
)

// ForwardExists check that at least 2 rules are present in the CNI-HOSTPORT-DNAT chain
// and checks for regex matches in the list of rules
func ForwardExists(t *testing.T, ipt *iptables.IPTables, chain, containerIP string, port int) bool {
	rules, err := ipt.List("nat", chain)
	if err != nil {
		t.Logf("error listing rules in chain: %q\n", err)
		return false
	}

	if len(rules) < 1 {
		t.Logf("not enough rules: %d", len(rules))
		return false
	}

	// here we check if at least one of the rules in the chain
	// matches the required string to identify that the rule was applied
	found := false
	matchRule := `--dport ` + fmt.Sprintf("%d", port) + ` .+ --to-destination ` + containerIP
	for _, rule := range rules {
		foundInRule, err := regexp.MatchString(matchRule, rule)
		if err != nil {
			t.Logf("error in match string: %q\n", err)
			return false
		}
		if foundInRule {
			found = foundInRule
		}
	}
	return found
}

// GetRedirectedChain returns the chain where the traffic is being redirected.
// This is how libcni manage its port maps.
// Suppose you have the following rule:
// -A CNI-HOSTPORT-DNAT -p tcp -m comment --comment "dnat name: \"bridge\" id: \"default-YYYYYY\"" -m multiport --dports 9999 -j CNI-DN-XXXXXX
// So the chain where the traffic is redirected is CNI-DN-XXXXXX
// Returns an empty string in case nothing was found.
func GetRedirectedChain(t *testing.T, ipt *iptables.IPTables, chain, namespace, containerID string) string {
	rules, err := ipt.List("nat", chain)
	if err != nil {
		t.Logf("error listing rules in chain: %q\n", err)
		return ""
	}

	if len(rules) < 1 {
		t.Logf("not enough rules: %d", len(rules))
		return ""
	}

	var redirectedChain string
	re := regexp.MustCompile(`-j\s+([^ ]+)`)
	for _, rule := range rules {
		// first we verify the comment section is present: "dnat name: \"bridge\" id: \"default-YYYYYY\""
		matchesContainer, err := regexp.MatchString(namespace+"-"+containerID, rule)
		if err != nil {
			t.Logf("error in match string: %q\n", err)
			return ""
		}
		if matchesContainer {
			// then we find the appropriate chain in the rule
			matches := re.FindStringSubmatch(rule)
			fmt.Println(matches)
			if len(matches) >= 2 {
				redirectedChain = matches[1]
			}
		}
	}
	if redirectedChain == "" {
		t.Logf("no redirectced chain found")
		return ""
	}
	return redirectedChain
}
