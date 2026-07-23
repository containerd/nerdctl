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
	"fmt"
	"net"

	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
)

func GetLiveNetworkSubnets() ([]*net.IPNet, error) {
	var addrs []net.Addr
	if err := rootlessutil.WithDetachedNetNSIfAny(func() error {
		var err2 error
		addrs, err2 = net.InterfaceAddrs()
		return err2
	}); err != nil {
		return nil, err
	}
	nets := make([]*net.IPNet, 0, len(addrs))
	for _, address := range addrs {
		_, n, err := net.ParseCIDR(address.String())
		if err != nil {
			return nil, err
		}
		nets = append(nets, n)
	}
	return nets, nil
}

// GetFreeSubnet try to find a free subnet in the given network
func GetFreeSubnet(n *net.IPNet, usedNetworks []*net.IPNet) (*net.IPNet, error) {
	for {
		if !IntersectsWithNetworks(n, usedNetworks) {
			return n, nil
		}
		next, err := nextSubnet(n)
		if err != nil {
			break
		}
		n = next
	}
	return nil, fmt.Errorf("could not find free subnet")
}

func nextSubnet(subnet *net.IPNet) (*net.IPNet, error) {
	newSubnet := &net.IPNet{
		IP:   subnet.IP,
		Mask: subnet.Mask,
	}
	ones, bits := newSubnet.Mask.Size()
	if ones == 0 {
		return nil, fmt.Errorf("%s has only one subnet", subnet.String())
	}
	zeroes := uint(bits - ones)
	shift := zeroes % 8
	idx := (ones - 1) / 8
	if err := incByte(newSubnet, idx, shift); err != nil {
		return nil, err
	}
	return newSubnet, nil
}

func incByte(subnet *net.IPNet, idx int, shift uint) error {
	if idx < 0 {
		return fmt.Errorf("no more subnets left")
	}

	var val byte = 1 << shift
	// if overflow we have to inc the previous byte
	if uint(subnet.IP[idx])+uint(val) > 255 {
		if err := incByte(subnet, idx-1, 0); err != nil {
			return err
		}
	}
	subnet.IP[idx] += val
	return nil
}

func IntersectsWithNetworks(n *net.IPNet, networklist []*net.IPNet) bool {
	for _, nw := range networklist {
		if n.Contains(nw.IP) || nw.Contains(n.IP) {
			return true
		}
	}
	return false
}

// lastIPInSubnet gets the last IP in a subnet
// https://github.com/containers/podman/blob/v4.0.0-rc1/libpod/network/util/ip.go#L18
func LastIPInSubnet(addr *net.IPNet) (net.IP, error) {
	// re-parse to ensure clean network address
	_, cidr, err := net.ParseCIDR(addr.String())
	if err != nil {
		return nil, err
	}
	ones, bits := cidr.Mask.Size()
	if ones == bits {
		return cidr.IP, nil
	}
	for i := range cidr.IP {
		cidr.IP[i] = cidr.IP[i] | ^cidr.Mask[i]
	}
	return cidr.IP, nil
}

// firstIPInSubnet gets the first IP in a subnet
// https://github.com/containers/podman/blob/v4.0.0-rc1/libpod/network/util/ip.go#L36
func FirstIPInSubnet(addr *net.IPNet) (net.IP, error) {
	// re-parse to ensure clean network address
	_, cidr, err := net.ParseCIDR(addr.String())
	if err != nil {
		return nil, err
	}
	ones, bits := cidr.Mask.Size()
	if ones == bits {
		return cidr.IP, nil
	}
	cidr.IP[len(cidr.IP)-1]++
	return cidr.IP, nil
}

// CIDRFromRange inverts FirstIPInSubnet/LastIPInSubnet: it rebuilds the CIDR from
// the start and end they produced. Returns "" for empty or unparsable bounds.
// A /31 or /127 has no distinct network and broadcast, so it recomputes as /32 or /128.
func CIDRFromRange(startStr, endStr string) string {
	if startStr == "" || endStr == "" {
		return ""
	}
	start, end := net.ParseIP(startStr), net.ParseIP(endStr)
	if start == nil || end == nil {
		return ""
	}
	// A single-address range is a /32 or /128.
	if start.Equal(end) {
		if start.To4() != nil {
			return start.String() + "/32"
		}
		return start.String() + "/128"
	}
	// Canonical byte form: 4 for v4, 16 for v6.
	s, e, bits := start.To4(), end.To4(), 32
	if s == nil {
		s, e, bits = start.To16(), end.To16(), 128
	}
	if e == nil || len(s) != len(e) {
		return ""
	}
	// Undo FirstIPInSubnet's last-byte bump to get the network.
	network := make(net.IP, len(s))
	copy(network, s)
	network[len(network)-1]--
	// end is the broadcast, so network^end is the host mask; its width is the host bits.
	hostBits := 0
	for i := range network {
		for b := network[i] ^ e[i]; b != 0; b >>= 1 {
			hostBits++
		}
	}
	return fmt.Sprintf("%s/%d", network.String(), bits-hostBits)
}
