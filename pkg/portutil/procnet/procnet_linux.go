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

package procnet

import (
	"bufio"
	"fmt"
	"os"
)

const (
	tcpProto  = "tcp"
	udpProto  = "udp"
	tcp6Proto = "tcp6"
	udp6Proto = "udp6"
	// FIXME: The /proc/net/tcp is not recommended by the kernel, FYI https://www.kernel.org/doc/Documentation/networking/proc_net_tcp.txt
	// In the future, we should use netlink instead of /proc/net/tcp
	netTcpStats  = "/proc/net/tcp"
	netUdpStats  = "/proc/net/udp"
	netTcp6Stats = "/proc/net/tcp6"
	netUdp6Stats = "/proc/net/udp6"
)

func ReadStatsFileData(protocol string) ([]string, error) {
	var fileAddress string

	if protocol == tcpProto {
		fileAddress = netTcpStats
	} else if protocol == udpProto {
		fileAddress = netUdpStats
	} else if protocol == tcp6Proto {
		fileAddress = netTcp6Stats
	} else if protocol == udp6Proto {
		fileAddress = netUdp6Stats
	} else {
		return nil, fmt.Errorf("Unknown protocol %s", protocol)
	}

	fp, err := os.Open(fileAddress)
	if err != nil {
		return nil, err
	}
	defer fp.Close()
	var lines []string
	sc := bufio.NewScanner(fp)

	for i := 0; sc.Scan(); i++ {
		if i == 0 {
			continue
		}
		lines = append(lines, sc.Text())
	}

	return lines, nil
}
