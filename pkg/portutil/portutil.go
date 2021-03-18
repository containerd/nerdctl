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

func ParseFlagP(s string) (*gocni.PortMapping, error) {
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

	res := &gocni.PortMapping{
		Protocol: proto,
		HostIP:   "0.0.0.0",
	}

	splitByColon := strings.Split(splitBySlash[0], ":")
	switch len(splitByColon) {
	case 1:
		return nil, errors.Errorf("automatic host port assignment is not supported yet (FIXME)")
	case 2:
		i, err := strconv.Atoi(splitByColon[0])
		if err != nil {
			return nil, err
		}
		res.HostPort = int32(i)
		i, err = strconv.Atoi(splitByColon[1])
		if err != nil {
			return nil, err
		}
		res.ContainerPort = int32(i)
		return res, nil
	case 3:
		res.HostIP = splitByColon[0]
		if net.ParseIP(res.HostIP) == nil {
			return nil, errors.Errorf("invalid IP %q", res.HostIP)
		}
		i, err := strconv.Atoi(splitByColon[1])
		if err != nil {
			return nil, err
		}
		res.HostPort = int32(i)
		i, err = strconv.Atoi(splitByColon[2])
		if err != nil {
			return nil, err
		}
		res.ContainerPort = int32(i)
		return res, nil
	default:
		return nil, errors.Errorf("failed to parse %q, unexpected colons", s)
	}
}
