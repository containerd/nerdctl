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
	"fmt"

	"github.com/containerd/nerdctl/pkg/portutil/procnet"
)

const (
	// This port range is compatible with Docker, FYI https://github.com/moby/moby/blob/eb9e42a09ee123af1d95bf7d46dd738258fa2109/libnetwork/portallocator/portallocator_unix.go#L7-L12
	allocateStart = 49153
	allocateEnd   = 60999
)

func filter(ss []procnet.NetworkDetail, filterFunc func(detail procnet.NetworkDetail) bool) (ret []procnet.NetworkDetail) {
	for _, s := range ss {
		if filterFunc(s) {
			ret = append(ret, s)
		}
	}
	return
}

func portAllocate(protocol string, ip string, count uint64) (uint64, uint64, error) {
	netprocData, err := procnet.ReadStatsFileData(protocol)
	if err != nil {
		return 0, 0, err
	}
	netprocItems := procnet.Parse(netprocData)
	// In some circumstances, when we bind address like "0.0.0.0:80", we will get the formation of ":::80" in /proc/net/tcp6.
	// So we need some trick to process this situation.
	if protocol == "tcp" {
		tempTCPV6Data, err := procnet.ReadStatsFileData("tcp6")
		if err != nil {
			return 0, 0, err
		}
		netprocItems = append(netprocItems, procnet.Parse(tempTCPV6Data)...)
	}
	if protocol == "udp" {
		tempUDPV6Data, err := procnet.ReadStatsFileData("udp6")
		if err != nil {
			return 0, 0, err
		}
		netprocItems = append(netprocItems, procnet.Parse(tempUDPV6Data)...)
	}
	if ip != "" {
		netprocItems = filter(netprocItems, func(s procnet.NetworkDetail) bool {
			// In some circumstances, when we bind address like "0.0.0.0:80", we will get the formation of ":::80" in /proc/net/tcp6.
			// So we need some trick to process this situation.
			return s.LocalIP.String() == "::" || s.LocalIP.String() == ip
		})
	}

	usedPort := make(map[uint64]bool)
	for _, value := range netprocItems {
		usedPort[value.LocalPort] = true
	}
	start := uint64(allocateStart)
	if count > uint64(allocateEnd-allocateStart+1) {
		return 0, 0, fmt.Errorf("can not allocate %d ports", count)
	}
	for start < allocateEnd {
		needReturn := true
		for i := start; i < start+count; i++ {
			if _, ok := usedPort[i]; ok {
				needReturn = false
				break
			}
		}
		if needReturn {
			return start, start + count - 1, nil
		}
		start += count
	}
	return 0, 0, fmt.Errorf("there is not enough %d free ports", count)
}
