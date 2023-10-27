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

package types

import (
	gocni "github.com/containerd/go-cni"
)

// NetworkOptions struct defining networking-related options.
type NetworkOptions struct {
	// NetworkSlice specifies the networking mode for the container, default is "bridge"
	NetworkSlice []string
	// MACAddress set container MAC address (e.g., 92:d0:c6:0a:29:33)
	MACAddress string
	// IPAddress set specific static IP address(es) to use
	IPAddress string
	// IP6Address set specific static IP6 address(es) to use
	IP6Address string
	// Hostname set container host name
	Hostname string
	// DNSServers set custom DNS servers
	DNSServers []string
	// DNSResolvConfOptions set DNS options
	DNSResolvConfOptions []string
	// DNSSearchDomains set custom DNS search domains
	DNSSearchDomains []string
	// AddHost add a custom host-to-IP mapping (host:ip)
	AddHost []string
	// UTS namespace to use
	UTSNamespace string
	// PortMappings specifies a list of ports to publish from the container to the host
	PortMappings []gocni.PortMapping
}
