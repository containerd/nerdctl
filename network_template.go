/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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

package main

// defaultBridgeNetwork is the default bridge network.
//
// The CIDR is 10.4.0.0/16 .
//
// The template was copied from https://github.com/containers/podman/blob/v2.2.0/cni/87-podman-bridge.conflist
const defaultBridgeNetwork = `{
  "cniVersion": "0.4.0",
  "name": "nerdctl",
  "plugins": [
    {
      "type": "bridge",
      "bridge": "cni-nerdctl0",
      "isGateway": true,
      "ipMasq": true,
      "hairpinMode": true,
      "ipam": {
        "type": "host-local",
        "routes": [{ "dst": "0.0.0.0/0" }],
        "ranges": [
          [
            {
              "subnet": "10.4.0.0/16",
              "gateway": "10.4.0.1"
            }
          ]
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
      "type": "firewall"
    },
    {
      "type": "tuning"
    }
  ]
}`
