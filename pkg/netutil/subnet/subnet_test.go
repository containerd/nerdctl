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

package subnet

import (
	"net"
	"testing"

	"gotest.tools/v3/assert"
)

func TestNextSubnet(t *testing.T) {
	testCases := []struct {
		subnet string
		expect string
	}{
		{
			subnet: "10.4.1.0/24",
			expect: "10.4.2.0/24",
		},
		{
			subnet: "10.4.255.0/24",
			expect: "10.5.0.0/24",
		},
		{
			subnet: "10.4.255.0/16",
			expect: "10.5.0.0/16",
		},
	}
	for _, tc := range testCases {
		_, net, _ := net.ParseCIDR(tc.subnet)
		nextSubnet, err := nextSubnet(net)
		assert.NilError(t, err)
		assert.Equal(t, nextSubnet.String(), tc.expect)
	}
}

func TestCIDRFromRange(t *testing.T) {
	testCases := []struct {
		name       string
		start, end string
		expect     string
	}{
		{"no range", "", "", ""},
		{"v4 /24", "10.1.100.1", "10.1.100.255", "10.1.100.0/24"},
		{"v4 /25", "10.24.24.1", "10.24.24.127", "10.24.24.0/25"},
		{"v4 /16", "172.28.0.1", "172.28.255.255", "172.28.0.0/16"},
		{"v4 offset /25", "10.1.100.129", "10.1.100.255", "10.1.100.128/25"},
		{"v4 /32", "10.0.0.5", "10.0.0.5", "10.0.0.5/32"},
		{"v4 /31 collapses to /32", "10.0.0.1", "10.0.0.1", "10.0.0.1/32"},
		{"v6 /64", "fd00:55::1", "fd00:55::ffff:ffff:ffff:ffff", "fd00:55::/64"},
		{"v6 /120", "fd00:7::1", "fd00:7::ff", "fd00:7::/120"},
		{"v6 /128", "fd00::5", "fd00::5", "fd00::5/128"},
		{"v6 /127 collapses to /128", "fd00::1", "fd00::1", "fd00::1/128"},
		{"start unparsable", "bogus", "10.0.0.255", ""},
		{"end unparsable", "10.0.0.1", "bogus", ""},
		{"mismatched families", "10.0.0.1", "fd00::ff", ""},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expect, CIDRFromRange(tc.start, tc.end))
		})
	}
}
