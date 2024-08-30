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

package helpers

import (
	"net"
	"strings"
)

func FindIPv6(output string) net.IP {
	var ipv6 string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "inet6") {
			fields := strings.Fields(line)
			if len(fields) > 1 {
				ipv6 = strings.Split(fields[1], "/")[0]
				break
			}
		}
	}
	return net.ParseIP(ipv6)
}
