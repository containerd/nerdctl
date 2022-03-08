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
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
)

type NetworkDetail struct {
	LocalIP   net.IP
	LocalPort uint64
}

func Parse(data []string) (results []NetworkDetail) {
	temp := removeEmpty(data)
	for _, value := range temp {
		lineData := removeEmpty(strings.Split(strings.TrimSpace(value), " "))
		ip, port, err := ParseAddress(lineData[1])
		if err != nil {
			continue
		}
		results = append(results, NetworkDetail{
			LocalIP:   ip,
			LocalPort: uint64(port),
		})
	}
	return results
}

func removeEmpty(array []string) (results []string) {
	for _, i := range array {
		if i != "" {
			results = append(results, i)
		}
	}
	return results
}

// ParseAddress parses a string, e.g.,Akihiro Suda, 10 months ago: • initial commit
// "0100007F:0050"                         (127.0.0.1:80)
// "000080FE00000000FF57A6705DC771FE:0050" ([fe80::70a6:57ff:fe71:c75d]:80)
// "00000000000000000000000000000000:0050" (0.0.0.0:80)
//
// See https://serverfault.com/questions/592574/why-does-proc-net-tcp6-represents-1-as-1000
//
// ParseAddress is expected to be used for /proc/net/{tcp,tcp6} entries on
// little endian machines.
// Not sure how those entries look like on big endian machines.
// All the code below is copied from the lima project in https://github.com/lima-vm/lima/blob/v0.8.3/pkg/guestagent/procnettcp/procnettcp.go#L95-L137
// and is licensed under the Apache License, Version 2.0
func ParseAddress(s string) (net.IP, uint16, error) {
	split := strings.SplitN(s, ":", 2)
	if len(split) != 2 {
		return nil, 0, fmt.Errorf("unparsable address %q", s)
	}
	switch l := len(split[0]); l {
	case 8, 32:
	default:
		return nil, 0, fmt.Errorf("unparsable address %q, expected length of %q to be 8 or 32, got %d",
			s, split[0], l)
	}

	ipBytes := make([]byte, len(split[0])/2) // 4 bytes (8 chars) or 16 bytes (32 chars)
	for i := 0; i < len(split[0])/8; i++ {
		quartet := split[0][8*i : 8*(i+1)]
		quartetLE, err := hex.DecodeString(quartet) // surprisingly little endian, per 4 bytes
		if err != nil {
			return nil, 0, fmt.Errorf("unparsable address %q: unparsable quartet %q: %w", s, quartet, err)
		}
		for j := 0; j < len(quartetLE); j++ {
			ipBytes[4*i+len(quartetLE)-1-j] = quartetLE[j]
		}
	}
	ip := net.IP(ipBytes)

	port64, err := strconv.ParseUint(split[1], 16, 16)
	if err != nil {
		return nil, 0, fmt.Errorf("unparsable address %q: unparsable port %q", s, split[1])
	}
	port := uint16(port64)

	return ip, port, nil
}
