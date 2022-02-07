//go:build freebsd || linux
// +build freebsd linux

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
	DefaultNetworkName = "bridge"
	DefaultID          = 0
	DefaultCIDR        = "10.4.0.0/24"
	DefaultIPAMDriver  = "host-local"
)

func GenerateCNIPlugins(driver string, id int, ipam map[string]interface{}, opts map[string]string) ([]CNIPlugin, error) {
	var (
		plugins []CNIPlugin
		err     error
	)
	switch driver {
	case "bridge":
		mtu := 0
		for opt, v := range opts {
			switch opt {
			case "mtu", "com.docker.network.driver.mtu":
				mtu, err = ParseMTU(v)
				if err != nil {
					return nil, err
				}
			default:
				return nil, fmt.Errorf("unsupported network option %q", opt)
			}
		}
		bridge := newBridgePlugin(GetBridgeName(id))
		bridge.MTU = mtu
		bridge.IPAM = ipam
		bridge.IsGW = true
		bridge.IPMasq = true
		bridge.HairpinMode = true
		// gVisor needs "loopback" plugin to be added explicitly
		plugins = []CNIPlugin{bridge, newPortMapPlugin(), newFirewallPlugin(), newTuningPlugin(), newLoopbackPlugin()}
	default:
		return nil, fmt.Errorf("unsupported cni driver %q", driver)
	}
	return plugins, nil
}

func GenerateIPAM(driver string, subnetStr, gatewayStr, ipRangeStr string) (map[string]interface{}, error) {
	if driver == "" {
		driver = DefaultIPAMDriver
	}

	ipamRange, err := parseIPAMRange(subnetStr, gatewayStr, ipRangeStr)
	if err != nil {
		return nil, err
	}

	var ipamConfig interface{}
	switch driver {
	case "host-local":
		ipamConf := newHostLocalIPAMConfig()
		ipamConf.Routes = []IPAMRoute{
			{Dst: "0.0.0.0/0"},
		}
		ipamConf.Ranges = append(ipamConf.Ranges, []IPAMRange{*ipamRange})
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
