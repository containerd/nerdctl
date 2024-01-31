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
		return cidr.IP, err
	}
	for i := range cidr.IP {
		cidr.IP[i] = cidr.IP[i] | ^cidr.Mask[i]
	}
	return cidr.IP, err
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
		return cidr.IP, err
	}
	cidr.IP[len(cidr.IP)-1]++
	return cidr.IP, err
}
