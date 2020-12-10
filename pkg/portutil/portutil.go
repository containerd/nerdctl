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

package portutil

import (
	"net"
	"strconv"
	"strings"

	gocni "github.com/containerd/go-cni"
	"github.com/pkg/errors"
)

func ParseFlagP(s string) (*gocni.PortMapping, error) {
	if strings.Contains(s, "/") && !strings.HasSuffix(s, "/tcp") {
		return nil, errors.New("non-TCP protocol is not implemented yet (FIXME)")
	}
	split := strings.Split(s, ":")
	res := &gocni.PortMapping{
		Protocol: "tcp",
		HostIP:   "0.0.0.0",
	}
	switch len(split) {
	case 2:
		i, err := strconv.Atoi(split[0])
		if err != nil {
			return nil, err
		}
		res.HostPort = int32(i)
		i, err = strconv.Atoi(split[1])
		if err != nil {
			return nil, err
		}
		res.ContainerPort = int32(i)
		return res, nil
	case 3:
		res.HostIP = split[0]
		if net.ParseIP(res.HostIP) == nil {
			return nil, errors.Errorf("invalid IP %q", res.HostIP)
		}
		i, err := strconv.Atoi(split[1])
		if err != nil {
			return nil, err
		}
		res.HostPort = int32(i)
		i, err = strconv.Atoi(split[2])
		if err != nil {
			return nil, err
		}
		res.ContainerPort = int32(i)
		return res, nil
	default:
		return nil, errors.Errorf("failed to parse %q", s)
	}
}
