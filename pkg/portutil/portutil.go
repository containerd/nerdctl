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
	"strconv"
	"strings"

	gocni "github.com/containerd/go-cni"
	"github.com/pkg/errors"
)

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

	multi_res := []gocni.PortMapping{}

	ip, hostPort, containerPort := splitParts(splitBySlash[0])

	if containerPort == "" {
		return nil, errors.Errorf("No port specified: %s<empty>", splitBySlash[0])
	}

	if hostPort == "" {
		return nil, errors.Errorf("automatic host port assignment is not supported yet (FIXME)")
	}

	i, err := strconv.Atoi(hostPort)
	if err != nil {
		return nil, err
	}
	res.HostPort = int32(i)

	i, err = strconv.Atoi(containerPort)
	if err != nil {
		return nil, err
	}
	res.ContainerPort = int32(i)

	if ip == "" {
		res.HostIP = "0.0.0.0"
		multi_res = append(multi_res, res)
		res.HostIP = "::"
		multi_res = append(multi_res, res)
	} else {
		if ip[0] == '[' {
			// Strip [] from IPV6 addresses
			rawIP, _, err := net.SplitHostPort(ip + ":")
			if err != nil {
				return nil, errors.Errorf("Invalid ip address %v: %s", ip, err)
			}
			ip = rawIP
		}

		if net.ParseIP(ip) == nil {
			return nil, errors.Errorf("Invalid ip address: %s", ip)
		}
		res.HostIP = ip
		multi_res = append(multi_res, res)
	}
	return multi_res, nil
}
