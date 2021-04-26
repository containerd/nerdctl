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
	"net"
	"strings"

	gocni "github.com/containerd/go-cni"
	"github.com/docker/go-connections/nat"
	"github.com/pkg/errors"
)

//return respectively ip, hostPort, containerPort
func splitParts(rawport string) (string, string, string) {
	parts := strings.Split(rawport, ":")
	n := len(parts)
	containerport := parts[n-1]

	switch n {
	case 1:
		return "", "", containerport
	case 2:
		return "", parts[0], containerport
	case 3:
		return parts[0], parts[1], containerport
	default:
		return strings.Join(parts[:n-2], ":"), parts[n-2], containerport
	}
}

func ParseFlagP(s string) ([]gocni.PortMapping, error) {
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
			return nil, errors.Errorf("invalid protocol %q", splitBySlash[1])
		}
	default:
		return nil, errors.Errorf("failed to parse %q, unexpected slashes", s)
	}

	res := gocni.PortMapping{
		Protocol: proto,
	}

	mr := []gocni.PortMapping{}

	ip, hostPort, containerPort := splitParts(splitBySlash[0])

	if containerPort == "" {
		return nil, errors.Errorf("no port specified: %s", splitBySlash[0])
	}

	if hostPort == "" {
		return nil, errors.Errorf("automatic host port assignment is not supported yet (FIXME)")
	}

	startHostPort, endHostPort, err := nat.ParsePortRange(hostPort)
	if err != nil {
		return nil, errors.Errorf("invalid hostPort: %s", hostPort)
	}

	startPort, endPort, err := nat.ParsePortRange(containerPort)
	if err != nil {
		return nil, errors.Errorf("invalid containerPort: %s", containerPort)
	}

	if hostPort != "" && (endPort-startPort) != (endHostPort-startHostPort) {
		if endPort != startPort {
			return nil, errors.Errorf("invalid ranges specified for container and host Ports: %s and %s", containerPort, hostPort)
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
				return nil, errors.Errorf("invalid ip address: %s", ip)
			}
			res.HostIP = ip
		}

		mr = append(mr, res)
	}

	return mr, nil
}
