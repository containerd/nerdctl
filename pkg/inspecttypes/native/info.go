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

package native

import (
	introspection "github.com/containerd/containerd/api/services/introspection/v1"
	version "github.com/containerd/containerd/api/services/version/v1"
)

type Info struct {
	Client *ClientInfo `json:"Client,omitempty"`
	Daemon *DaemonInfo `json:"Daemon,omitempty"`
}

type ClientInfo struct {
	Namespace     string `json:"Namespace,omitempty"`
	Snapshotter   string `json:"Snapshotter,omitempty"`
	CgroupManager string `json:"CgroupManager,omitempty"`
}

type DaemonInfo struct {
	Plugins *introspection.PluginsResponse `json:"Plugins,omitempty"`
	Server  *introspection.ServerResponse  `json:"Server,omitempty"`
	Version *version.VersionResponse       `json:"Version,omitempty"`
}
