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
	"errors"
	"fmt"
	"net"

	"github.com/go-viper/mapstructure/v2"
)

const (
	DefaultNetworkName = "nat"
	DefaultCIDR        = "10.4.0.0/24"

	// When creating non-default network without passing in `--subnet` option,
	// nerdctl assigns subnet address for the creation starting from `StartingCIDR`
	// This prevents subnet address overlapping with `DefaultCIDR` used by the default network
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

func (e *CNIEnv) generateCNIPlugins(driver string, name string, ipam map[string]interface{}, opts map[string]string, ipv6 bool, internal bool) ([]CNIPlugin, error) {
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

func (e *CNIEnv) generateIPAM(driver string, subnets []string, gateways []string, ipRanges []string, auxAddresses []string, opts map[string]string, ipv6, ipv4, internal bool) (map[string]interface{}, map[string]map[string]string, error) {
	switch driver {
	case "default":
	default:
		return nil, nil, fmt.Errorf("unsupported ipam driver %q", driver)
	}
	// IPv6-only networks are not supported on Windows.
	if !ipv4 {
		return nil, nil, fmt.Errorf("--ipv4=false is not supported on Windows")
	}
	// The Windows nat IPAM has no way to reserve individual addresses, so there
	// are never any aux-addresses to hand back to the caller.
	if len(auxAddresses) > 0 {
		return nil, nil, fmt.Errorf("--aux-address is not supported on Windows")
	}

	// Windows is single-subnet, so use at most one gateway and one ip-range.
	gatewayStr := ""
	if len(gateways) > 0 {
		gatewayStr = gateways[0]
	}
	ipRangeStr := ""
	if len(ipRanges) > 0 {
		ipRangeStr = ipRanges[0]
	}

	ipamConfig := newWindowsIPAMConfig()
	subnet, err := e.parseSubnet(subnets[0])
	if err != nil {
		return nil, nil, err
	}
	ipamRange, err := parseIPAMRange(subnet, gatewayStr, ipRangeStr)
	if err != nil {
		return nil, nil, err
	}
	ipamConfig.Subnet = ipamRange.Subnet
	ipamConfig.Routes = append(ipamConfig.Routes, IPAMRoute{Gateway: ipamRange.Gateway})
	ipam, err := structToMap(ipamConfig)
	if err != nil {
		return nil, nil, err
	}
	return ipam, nil, nil
}

func FirewallPluginGEQVersion(firewallPath string, versionStr string) (bool, error) {
	return false, errors.New("unsupported in windows")
}
