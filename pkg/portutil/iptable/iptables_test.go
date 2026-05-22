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
	"testing"
)

func TestParseIPTableRules(t *testing.T) {
	testCases := []struct {
		name  string
		rules []string
		want  []PortRule
	}{
		{
			name:  "Empty input",
			rules: []string{},
			want:  []PortRule{},
		},
		{
			name: "Single rule with single port",
			rules: []string{
				"-A CNI-HOSTPORT-DNAT -p tcp -m comment --comment \"dnat name: \"bridge\" id: \"some-id\"\" -m multiport --dports 8080 -j CNI-DN-some-hash",
			},
			want: []PortRule{{IP: "", Port: 8080}},
		},
		{
			name: "Multiple rules with multiple ports",
			rules: []string{
				"-A CNI-HOSTPORT-DNAT -p tcp -m comment --comment \"dnat name: \"bridge\" id: \"some-id\"\" -m multiport --dports 8080 -j CNI-DN-some-hash",
				"-A CNI-HOSTPORT-DNAT -p tcp -m comment --comment \"dnat name: \"bridge\" id: \"some-id\"\" -m multiport --dports 9090 -j CNI-DN-some-hash",
			},
			want: []PortRule{
				{IP: "", Port: 8080},
				{IP: "", Port: 9090},
			},
		},
		{
			name: "Single rule with comma-separated ports",
			rules: []string{
				"-A CNI-HOSTPORT-DNAT -p tcp -m comment --comment \"dnat name: \"bridge\" id: \"some-id\"\" -m multiport --dports 8080,9090 -j CNI-DN-some-hash",
			},
			want: []PortRule{
				{IP: "", Port: 8080},
				{IP: "", Port: 9090},
			},
		},
		{
			name: "Sub-chain DNAT rule with destination IP",
			rules: []string{
				"-A CNI-DN-some-hash -d 192.168.1.141/32 -p tcp -m tcp --dport 80 -j DNAT --to-destination 10.4.0.2:80",
			},
			want: []PortRule{{IP: "192.168.1.141", Port: 80}},
		},
		{
			name: "Multiple sub-chain rules with different IPs same port",
			rules: []string{
				"-A CNI-DN-hash1 -d 192.168.1.141/32 -p tcp -m tcp --dport 80 -j DNAT --to-destination 10.4.0.2:80",
				"-A CNI-DN-hash2 -d 192.168.1.142/32 -p tcp -m tcp --dport 80 -j DNAT --to-destination 10.4.0.3:80",
			},
			want: []PortRule{
				{IP: "192.168.1.141", Port: 80},
				{IP: "192.168.1.142", Port: 80},
			},
		},
		{
			name: "Sub-chain rule without CIDR suffix",
			rules: []string{
				"-A CNI-DN-hash1 -d 10.0.0.1 -p tcp -m tcp --dport 443 -j DNAT --to-destination 10.4.0.2:443",
			},
			want: []PortRule{{IP: "10.0.0.1", Port: 443}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseIPTableRules(tc.rules)
			if !equal(got, tc.want) {
				t.Errorf("ParseIPTableRules(%v) = %v; want %v", tc.rules, got, tc.want)
			}
		})
	}
}

func equal(a, b []PortRule) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
