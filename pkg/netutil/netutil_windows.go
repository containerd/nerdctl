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

	"github.com/mitchellh/mapstructure"
)

const (
	DefaultNetworkName = "nat"
	DefaultCIDR        = "10.4.0.0/24"
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

func (e *CNIEnv) generateCNIPlugins(driver string, name string, ipam map[string]interface{}, opts map[string]string) ([]CNIPlugin, error) {
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

func (e *CNIEnv) generateIPAM(driver string, subnetStr, gatewayStr, ipRangeStr string, opts map[string]string) (map[string]interface{}, error) {
	subnet, err := e.parseSubnet(subnetStr)
	if err != nil {
		return nil, err
	}
	ipamRange, err := parseIPAMRange(subnet, gatewayStr, ipRangeStr)
	if err != nil {
		return nil, err
	}

	var ipamConfig interface{}
	switch driver {
	case "default":
		ipamConf := newWindowsIPAMConfig()
		ipamConf.Subnet = ipamRange.Subnet
		ipamConf.Routes = append(ipamConf.Routes, IPAMRoute{Gateway: ipamRange.Gateway})
		ipamConfig = ipamConf
	default:
		return nil, fmt.Errorf("unsupported ipam driver %q", driver)
	}

	ipam, err := structToMap(ipamConfig)
	if err != nil {
		return nil, err
	}
	return ipam, nil
}

func removeBridgeNetworkInterface(name string) error {
	return nil
}
