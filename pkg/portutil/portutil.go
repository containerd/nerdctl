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
	"strings"

	"github.com/docker/go-connections/nat"

	"github.com/containerd/go-cni"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/portutil/portstore"
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
		var usedPorts map[uint64]bool
		usedPorts, err = getUsedPorts(ip, proto)
		if err != nil {
			return nil, err
		}
		for i := startHostPort; i <= endHostPort; i++ {
			if usedPorts[i] {
				return nil, fmt.Errorf("bind for %s:%d failed: port is already allocated", ip, i)
			}
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

func GeneratePortMappingsConfig(dataStore, namespace, id string, portMappings []cni.PortMapping) error {
	ps, err := portstore.New(dataStore, namespace, id)
	if err != nil {
		return err
	}
	return ps.Acquire(portMappings)
}

func LoadPortMappings(dataStore, namespace, id string, containerLabels map[string]string) ([]cni.PortMapping, error) {
	var ports []cni.PortMapping

	ps, err := portstore.New(dataStore, namespace, id)
	if err != nil {
		return ports, err
	}
	if err = ps.Load(); err != nil {
		return ports, err
	}
	if len(ps.PortMappings) != 0 {
		return ps.PortMappings, nil
	}

	portsJSON := containerLabels[labels.Ports]
	if portsJSON == "" {
		return ports, nil
	}
	if err := json.Unmarshal([]byte(portsJSON), &ports); err != nil {
		return ports, fmt.Errorf("failed to parse label %q=%q: %s", labels.Ports, portsJSON, err.Error())
	}
	log.L.Warnf("container %s (%s) is using legacy port mapping configuration. To ensure compatibility with the new port mapping logic, please recreate this container. For more details, see: https://github.com/containerd/nerdctl/pull/4290", containerLabels[labels.Name], id[:12])
	return ports, nil
}
