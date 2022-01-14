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

type CNIPlugin interface {
	GetPluginType() string
}

type IPAMRange struct {
	Subnet     string `json:"subnet"`
	RangeStart string `json:"rangeStart,omitempty"`
	RangeEnd   string `json:"rangeEnd,omitempty"`
	Gateway    string `json:"gateway,omitempty"`
	IPRange    string `json:"ipRange,omitempty"`
}

type IPAMRoute struct {
	Dst     string `json:"dst,omitempty"`
	GW      string `json:"gw,omitempty"`
	Gateway string `json:"gateway,omitempty"`
}

type isolationConfig struct {
	PluginType string `json:"type"`
}

func newIsolationPlugin() *isolationConfig {
	return &isolationConfig{
		PluginType: "isolation",
	}
}

func (*isolationConfig) GetPluginType() string {
	return "isolation"
}
