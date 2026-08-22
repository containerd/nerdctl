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

package containerinspector

import (
	"net"
	"testing"

	"github.com/vishvananda/netlink"
	"gotest.tools/v3/assert"
)

// defaultRoute mimics what netlink returns for a default route: Dst is not nil,
// it is synthesized as 0.0.0.0/0.
func defaultRoute(linkIndex int, gw string) netlink.Route {
	return netlink.Route{
		LinkIndex: linkIndex,
		Dst:       &net.IPNet{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)},
		Gw:        net.ParseIP(gw),
	}
}

func subnetRoute(linkIndex int, cidr string) netlink.Route {
	_, dst, _ := net.ParseCIDR(cidr)
	return netlink.Route{LinkIndex: linkIndex, Dst: dst}
}

func TestSelectDefaultGateway(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		routes   []netlink.Route
		primary  int
		expected string
	}{
		{
			name:     "default route on the primary interface",
			routes:   []netlink.Route{subnetRoute(2, "10.4.1.0/24"), defaultRoute(2, "10.4.1.1")},
			primary:  2,
			expected: "10.4.1.1",
		},
		{
			name:     "prefers the primary interface over another default route",
			routes:   []netlink.Route{defaultRoute(3, "192.168.0.1"), defaultRoute(2, "10.4.1.1")},
			primary:  2,
			expected: "10.4.1.1",
		},
		{
			name:     "falls back when no default route matches the primary interface",
			routes:   []netlink.Route{defaultRoute(3, "192.168.0.1")},
			primary:  2,
			expected: "192.168.0.1",
		},
		{
			name:     "nil Dst is still treated as a default route",
			routes:   []netlink.Route{{LinkIndex: 2, Gw: net.ParseIP("10.4.1.1")}},
			primary:  2,
			expected: "10.4.1.1",
		},
		{
			name:     "no default route",
			routes:   []netlink.Route{subnetRoute(2, "10.4.1.0/24")},
			primary:  2,
			expected: "",
		},
		{
			name:     "onlink default route without a gateway is skipped",
			routes:   []netlink.Route{{LinkIndex: 2, Dst: &net.IPNet{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)}}},
			primary:  2,
			expected: "",
		},
		{
			name:     "IPv6 gateway is ignored",
			routes:   []netlink.Route{{LinkIndex: 2, Gw: net.ParseIP("fe80::1")}},
			primary:  2,
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, selectDefaultGateway(tc.routes, tc.primary))
		})
	}
}
