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

package netutil

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/go-viper/mapstructure/v2"
)

const (
	DefaultNetworkName = "nat"
	DefaultCIDR        = "10.4.0.0/24"

	// When creating non-default network without passing in `--subnet` option,
	// nerdctl assigns subnet address for the creation starting from `StartingCIDR`
	// This prevents subnet address overlapping with `DefaultCIDR` used by the default networkß
	StartingCIDR = "10.4.1.0/24"
)

func (n *NetworkConfig) subnets() []*net.IPNet {
	var subnets []*net.IPNet
	if n.Plugins[0].Network.Type == "nat" {
		var nat natConfig
		if err := json.Unmarshal(n.Plugins[0].Bytes, &nat); err != nil {
			return subnets
		}
		var ipam windowsIpamConfig
		if err := mapstructure.Decode(nat.IPAM, &ipam); err != nil {
			return subnets
		}
		_, subnet, err := net.ParseCIDR(ipam.Subnet)
		if err != nil {
			return subnets
		}
		subnets = append(subnets, subnet)
	}
	return subnets
}

func (n *NetworkConfig) clean() error {
	return nil
}

func (e *CNIEnv) generateCNIPlugins(driver string, name string, ipam map[string]interface{}, opts map[string]string, ipv6 bool) ([]CNIPlugin, error) {
	var plugins []CNIPlugin
	switch driver {
	case "nat":
		nat := newNatPlugin("Ethernet")
		nat.IPAM = ipam
		plugins = []CNIPlugin{nat}
	default:
		return nil, fmt.Errorf("unsupported cni driver %q", driver)
	}
	return plugins, nil
}

func (e *CNIEnv) generateIPAM(driver string, subnets []string, gatewayStr, ipRangeStr string, opts map[string]string, ipv6 bool) (map[string]interface{}, error) {
	switch driver {
	case "default":
	default:
		return nil, fmt.Errorf("unsupported ipam driver %q", driver)
	}

	ipamConfig := newWindowsIPAMConfig()
	subnet, err := e.parseSubnet(subnets[0])
	if err != nil {
		return nil, err
	}
	ipamRange, err := parseIPAMRange(subnet, gatewayStr, ipRangeStr)
	if err != nil {
		return nil, err
	}
	ipamConfig.Subnet = ipamRange.Subnet
	ipamConfig.Routes = append(ipamConfig.Routes, IPAMRoute{Gateway: ipamRange.Gateway})
	ipam, err := structToMap(ipamConfig)
	if err != nil {
		return nil, err
	}
	return ipam, nil
}
