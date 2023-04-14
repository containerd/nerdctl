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

// Struct defining networking-related options.
type NetworkOptions struct {
	// --net/--network=<net name> ...
	NetworkSlice []string

	// --mac-address=<MAC>
	MACAddress string

	// --ip=<container static IP>
	IPAddress string

	// -h/--hostname=<container Hostname>
	Hostname string

	// --dns=<DNS host> ...
	DNSServers []string

	// --dns-opt/--dns-option=<resolv.conf line> ...
	DNSResolvConfOptions []string

	// --dns-search=<domain name> ...
	DNSSearchDomains []string

	// --add-host=<host:IP> ...
	AddHost []string

	// --uts=<Unix Time Sharing namespace>
	UTSNamespace string

	// -p/--publish=127.0.0.1:80:8080/tcp ...
	PortMappings []gocni.PortMapping
}
