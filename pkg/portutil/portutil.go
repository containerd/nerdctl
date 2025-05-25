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

package portutil

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/go-connections/nat"

	"github.com/containerd/go-cni"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
)

// return respectively ip, hostPort, containerPort
func splitParts(rawport string) (string, string, string) {
	lastIndex := strings.LastIndex(rawport, ":")
	containerPort := rawport[lastIndex+1:]
	if lastIndex == -1 {
		return "", "", containerPort
	}

	hostAddrPort := rawport[:lastIndex]
	addr, port, err := net.SplitHostPort(hostAddrPort)
	if err != nil {
		return "", hostAddrPort, containerPort
	}

	return addr, port, containerPort
}

// ParseFlagP parse port mapping pair, like "127.0.0.1:3000:8080/tcp",
// "127.0.0.1:3000-3001:8080-8081/tcp" and "3000:8080" ...
func ParseFlagP(s string) ([]cni.PortMapping, error) {
	proto := "tcp"
	splitBySlash := strings.Split(s, "/")
	switch len(splitBySlash) {
	case 1:
	// NOP
	case 2:
		proto = strings.ToLower(splitBySlash[1])
		switch proto {
		case "tcp", "udp", "sctp":
		default:
			return nil, fmt.Errorf("invalid protocol %q", splitBySlash[1])
		}
	default:
		return nil, fmt.Errorf("failed to parse %q, unexpected slashes", s)
	}

	res := cni.PortMapping{
		Protocol: proto,
	}

	mr := []cni.PortMapping{}

	ip, hostPort, containerPort := splitParts(splitBySlash[0])

	if containerPort == "" {
		return nil, fmt.Errorf("no port specified: %s", splitBySlash[0])
	}
	var startHostPort uint64
	var endHostPort uint64

	startPort, endPort, err := nat.ParsePortRange(containerPort)
	if err != nil {
		return nil, fmt.Errorf("invalid containerPort: %s", containerPort)
	}
	if hostPort == "" {
		// AutoHostPort could not be supported in rootless mode right now, because we can't get correct network from /proc/net/*
		if rootlessutil.IsRootless() {
			return nil, fmt.Errorf("automatic port allocation is not implemented for rootless mode (Hint: specify the port like \"12345:%s\", not just \"%s\")",
				containerPort, containerPort)
		}
		startHostPort, endHostPort, err = portAllocate(proto, ip, endPort-startPort+1)
		if err != nil {
			return nil, err
		}
		log.L.Debugf("There is no hostPort has been spec in command, the auto allocate port is from %d:%d to %d:%d", startHostPort, startPort, endHostPort, endPort)
	} else {
		startHostPort, endHostPort, err = nat.ParsePortRange(hostPort)
		if err != nil {
			return nil, fmt.Errorf("invalid hostPort: %s", hostPort)
		}
	}
	if hostPort != "" && (endPort-startPort) != (endHostPort-startHostPort) {
		if endPort != startPort {
			return nil, fmt.Errorf("invalid ranges specified for container and host Ports: %s and %s", containerPort, hostPort)
		}
	}

	for i := int32(0); i <= (int32(endPort) - int32(startPort)); i++ {

		res.ContainerPort = int32(startPort) + i
		res.HostPort = int32(startHostPort) + i
		if ip == "" {
			//TODO handle ipv6
			res.HostIP = "0.0.0.0"
		} else {
			// TODO handle ipv6
			if net.ParseIP(ip) == nil {
				return nil, fmt.Errorf("invalid ip address: %s", ip)
			}
			res.HostIP = ip
		}

		mr = append(mr, res)
	}

	return mr, nil
}

func portMappingsPath(dataStore, ns, id string) string {
	return filepath.Join(dataStore, "containers", ns, id, "port-mappings.json")
}

func GeneratePortMappingsConfig(dataStore, ns, id string, portMappings []cni.PortMapping) error {
	portsJSON, err := json.Marshal(portMappings)
	if err != nil {
		return fmt.Errorf("failed to marshal port mappings to JSON: %w", err)
	}
	portMappingsPath := portMappingsPath(dataStore, ns, id)
	if err := os.WriteFile(portMappingsPath, portsJSON, 0644); err != nil {
		return fmt.Errorf("failed to write %s to %s: %w", portMappingsPath, string(portsJSON), err)
	}
	return nil
}

func LoadPortMappingsData(dataStore, ns, id string) (string, error) {
	portMappingsPath := portMappingsPath(dataStore, ns, id)
	if _, err := os.Stat(portMappingsPath); err != nil {
		return "", nil
	}
	portMappingsData, err := os.ReadFile(portMappingsPath)
	if err != nil {
		return "", fmt.Errorf("failed to read portMappings file %q: %w", portMappingsData, err)
	}
	return string(portMappingsData), err
}

// ParsePortsLabel parses a JSON string containing port specifications
// like [{"HostPort":80,"ContainerPort":80,"Protocol":"tcp","HostIP":"0.0.0.0"}]
// and returns a slice of cni.PortMapping.
func ParsePortsLabel(portsJSON string) ([]cni.PortMapping, error) {
	var ports []cni.PortMapping
	if portsJSON == "" {
		return ports, nil
	}
	if err := json.Unmarshal([]byte(portsJSON), &ports); err != nil {
		return ports, fmt.Errorf("failed to parse port mappings %s: %w", portsJSON, err)
	}
	return ports, nil
}

func LoadPortMappings(dataStore, ns, id string) ([]cni.PortMapping, error) {
	portMappingsData, err := LoadPortMappingsData(dataStore, ns, id)
	if err != nil {
		return []cni.PortMapping{}, err
	}
	portMappings, err := ParsePortsLabel(portMappingsData)
	if err != nil {
		return []cni.PortMapping{}, fmt.Errorf("failed to read portMappings file %q: %w", portMappingsData, err)
	}
	return portMappings, nil
}
