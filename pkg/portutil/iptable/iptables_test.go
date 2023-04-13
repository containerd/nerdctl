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
		want  []uint64
	}{
		{
			name:  "Empty input",
			rules: []string{},
			want:  []uint64{},
		},
		{
			name: "Single rule with single port",
			rules: []string{
				"-A CNI-HOSTPORT-DNAT -p tcp -m comment --comment \"dnat name: \"bridge\" id: \"some-id\"\" -m multiport --dports 8080 -j CNI-DN-some-hash",
			},
			want: []uint64{8080},
		},
		{
			name: "Multiple rules with multiple ports",
			rules: []string{
				"-A CNI-HOSTPORT-DNAT -p tcp -m comment --comment \"dnat name: \"bridge\" id: \"some-id\"\" -m multiport --dports 8080 -j CNI-DN-some-hash",
				"-A CNI-HOSTPORT-DNAT -p tcp -m comment --comment \"dnat name: \"bridge\" id: \"some-id\"\" -m multiport --dports 9090 -j CNI-DN-some-hash",
			},
			want: []uint64{8080, 9090},
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

func equal(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}
