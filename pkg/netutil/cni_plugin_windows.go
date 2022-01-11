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

type natConfig struct {
	PluginType string                 `json:"type"`
	Master     string                 `json:"master,omitempty"`
	IPAM       map[string]interface{} `json:"ipam"`
}

func (*natConfig) GetPluginType() string {
	return "nat"
}

func newNatPlugin(master string) *natConfig {
	return &natConfig{
		PluginType: "nat",
		Master:     master,
	}
}

// https://github.com/microsoft/windows-container-networking/blob/v0.2.0/cni/cni.go#L55-L63
type windowsIpamConfig struct {
	Type          string      `json:"type"`
	Environment   string      `json:"environment,omitempty"`
	AddrSpace     string      `json:"addressSpace,omitempty"`
	Subnet        string      `json:"subnet,omitempty"`
	Address       string      `json:"ipAddress,omitempty"`
	QueryInterval string      `json:"queryInterval,omitempty"`
	Routes        []IPAMRoute `json:"routes,omitempty"`
}

func newWindowsIPAMConfig() *windowsIpamConfig {
	return &windowsIpamConfig{}
}
