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

const (
	DefaultNetworkName = "nat"
	DefaultID          = 0
	DefaultCIDR        = "10.1.0.0/24"
)

// basicPlugins is used by ConfigListTemplate
var basicPlugins = []string{"nat"}

// ConfigListTemplate was copied from https://github.com/containers/podman/blob/v2.2.0/cni/87-podman-bridge.conflist

const ConfigListTemplate = `{
  "cniVersion": "0.4.0",
  "name": "{{.Name}}",
  "nerdctlID": {{.ID}},
  "plugins": [
    {
      "type": "nat",
      "master": "Ethernet",
      "ipam": {
        "subnet": "{{.Subnet}}",
        "routes": [
            {
                "gateway": "{{.Gateway}}"
            }
        ]
      }
    },
    {
      "type": "portmap",
      "capabilities": {
        "portMappings": true
      }
    },
    {
      "type": "tuning"
    }{{.ExtraPlugins}}
  ]
}`

