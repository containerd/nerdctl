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

func GenerateCNIPlugins(driver string, id int, ipam map[string]interface{}, opts map[string]string) ([]CNIPlugin, error) {
	return nil, fmt.Errorf("unsupported cni driver %q on darwin", driver)
}

func GenerateIPAM(driver string, subnetStr, gatewayStr, ipRangeStr string) (map[string]interface{}, error) {
	// TODO: silence golangcli for unused function
	_, err := structToMap(newDarwinIPAMConfig())
	if err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("unsupported ipam driver %q on darwin", driver)
}
