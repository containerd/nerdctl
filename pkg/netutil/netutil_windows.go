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
	"fmt"
)

const (
	DefaultNetworkName = "nat"
	DefaultID          = 0
	DefaultCIDR        = "10.4.0.0/24"
	DefaultCNIPlugin   = "nat"
)

func GenerateCNIPlugins(driver string, id int, ipam map[string]interface{}) ([]CNIPlugin, error) {
	if driver == "" {
		driver = DefaultCNIPlugin
	}
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

func GenerateIPAM(driver string, subnetStr string) (map[string]interface{}, error) {
	subnet, gateway, err := parseSubnet(subnetStr)
	if err != nil {
		return nil, err
	}

	var ipamConfig interface{}
	switch driver {
	case "":
		ipamConf := newWindowsIPAMConfig()
		ipamConf.Subnet = subnet.String()
		ipamConf.Routes = append(ipamConf.Routes, IPAMRoute{Gateway: gateway.String()})
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
