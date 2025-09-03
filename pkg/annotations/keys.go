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

// Package annotations defines OCI annotations
package annotations

const (
	// prefix is the common prefix of nerdctl annotations
	prefix = "nerdctl/"

	// AnonymousVolumes is a JSON-marshalled string of []string
	AnonymousVolumes = prefix + "anonymous-volumes"

	// Bypass4netns is the flag for acceleration with bypass4netns
	// Boolean value which can be parsed with strconv.ParseBool() is required.
	// (like "nerdctl/bypass4netns=true" or "nerdctl/bypass4netns=false")
	Bypass4netns = prefix + "bypass4netns"

	// Bypass4netnsIgnoreSubnets is a JSON of []string that is appended to
	// the `bypass4netns --ignore` list.
	Bypass4netnsIgnoreSubnets = Bypass4netns + "-ignore-subnets"

	// Bypass4netnsIgnoreBind disables acceleration for bind.
	// Boolean value which can be parsed with strconv.ParseBool() is required.
	Bypass4netnsIgnoreBind = Bypass4netns + "-ignore-bind"

	// ContainerAutoRemove is to check whether the --rm option is specified.
	ContainerAutoRemove = prefix + "auto-remove"

	// Domainname
	Domainname = prefix + "domainname"

	// DNSSettings sets the dockercompat DNS config values
	DNSSetting = prefix + "dns"

	// ExtraHosts are HostIPs to appended to /etc/hosts
	ExtraHosts = prefix + "extraHosts"

	// HostConfig sets the dockercompat host config values
	HostConfig = prefix + "host-config"

	// Hostname
	Hostname = prefix + "hostname"

	// IP6Address is the static IP6 address of the container assigned by the user
	IP6Address = prefix + "ip6"

	// IPAddress is the static IP address of the container assigned by the user
	IPAddress = prefix + "ip"

	// IPC is the `nerectl run --ipc` for restrating
	// IPC indicates ipc victim container.
	IPC = prefix + "ipc"

	// LogConfig defines the logging configuration passed to the container
	LogConfig = prefix + "log-config"

	// LogURI is the log URI
	LogURI = prefix + "log-uri"

	MACAddress = prefix + "mac-address"

	// Mounts is the mount points for the container.
	Mounts = prefix + "mounts"

	Name = prefix + "name"

	// Namespace is the containerd namespace such as "default", "k8s.io"
	Namespace = prefix + "namespace"

	// NetworkNamespace is the network namespace path to be passed to the CNI plugins.
	// When this annotation is set from the runtime spec.State payload, it takes
	// precedence over the PID based resolution (/proc/<pid>/ns/net) where pid is
	// spec.State.Pid.
	// This is mostly used for VM based runtime, where the spec.State PID does not
	// necessarily lives in the created container networking namespace.
	//
	// On Windows, this label will contain the UUID of a namespace managed by
	// the Host Compute Network Service (HCN) API.
	NetworkNamespace = prefix + "network-namespace"

	// Networks is a JSON-marshalled string of []string, e.g. []string{"bridge"}.
	// Currently, the length of the slice must be 1.
	Networks = prefix + "networks"

	// PIDContainer is the `nerdctl run --pid` for restarting
	PIDContainer = prefix + "pid-container"

	// PIDFile is the `nerdctl run --pidfile`
	// (CLI flag is "pidfile", not "pid-file", for Podman compatibility)
	PIDFile = prefix + "pid-file"

	// Platform is the normalized platform string like "linux/ppc64le".
	Platform = prefix + "platform"

	// Ports is a JSON-marshalled string of []cni.PortMapping .
	Ports = prefix + "ports"

	// StateDir is "/var/lib/nerdctl/<ADDRHASH>/containers/<NAMESPACE>/<ID>"
	StateDir = prefix + "state-dir"

	// User is the username of the container
	User = prefix + "user"
)

var ShellCompletions = []string{
	Bypass4netns + "=true",
	Bypass4netns + "=false",
	Bypass4netnsIgnoreSubnets + "=",
	Bypass4netnsIgnoreBind + "=true",
	Bypass4netnsIgnoreBind + "=false",
}
