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

package native

import (
	"net"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
)

// Container corresponds to a containerd-native container object.
// Not compatible with `docker container inspect`.
type Container struct {
	containers.Container
	Spec    interface{} `json:"Spec,omitempty"`
	Process *Process    `json:"Process,omitempty"`
}

type Process struct {
	Pid    int               `json:"Pid,omitempty"`
	Status containerd.Status `json:"Status,omitempty"`
	NetNS  *NetNS            `json:"NetNS,omitempty"`
}

// NetNS is designed not to depend on CNI
type NetNS struct {
	// PrimaryInterface is a net.Interface.Index value, not an array index.
	// Zero means unset.
	PrimaryInterface int            `json:"PrimaryInterface,omitempty"`
	Interfaces       []NetInterface `json:"Interfaces,omitempty"`
}

// NetInteface wraps net.Interface for JSON marshallability.
// No support for unmarshalling.
type NetInterface struct {
	net.Interface
	// HardwareAddr overrides Interface.HardwareAddr
	HardwareAddr string
	// Flags overrides Interface.Flags
	Flags []string
	Addrs []string
}
