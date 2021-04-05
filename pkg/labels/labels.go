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

// Package labels defines labels that are set to containerd containers as labels.
// The labels are also passed to OCI containers as annotations.
package labels

const (
	// Prefix is the common prefix of nerdctl labels
	Prefix = "nerdctl/"

	// Namespace is the containerd namespace such as "default", "k8s.io"
	Namespace = Prefix + "namespace"

	// Name is a human-friendly name.
	// WARNING: multiple containers may have same the name label
	Name = Prefix + "name"

	// Hostname
	Hostname = Prefix + "hostname"

	// StateDir is "/var/lib/nerdctl/<ADDRHASH>/containers/<NAMESPACE>/<ID>"
	StateDir = Prefix + "state-dir"

	// Networks is a JSON-marshalled string of []string, e.g. []string{"bridge"}.
	// Currently, the length of the slice must be 1.
	Networks = Prefix + "networks"

	// Ports is a JSON-marshalled string of []gocni.PortMapping .
	Ports = Prefix + "ports"

	// LogURI is the log URI
	LogURI = Prefix + "log-uri"

	// AnonymousVolumes is a JSON-marshalled string of []string
	AnonymousVolumes = Prefix + "anonymous-volumes"
)
