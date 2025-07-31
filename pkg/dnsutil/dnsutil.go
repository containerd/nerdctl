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

package dnsutil

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
)

func GetSlirp4netnsDNS() ([]string, error) {
	var dns []string
	rkClient, err := rootlessutil.NewRootlessKitClient()
	if err != nil {
		return dns, err
	}
	info, err := rkClient.Info(context.TODO())
	if err != nil {
		return dns, err
	}
	if info != nil && info.NetworkDriver != nil {
		for _, dnsIP := range info.NetworkDriver.DNS {
			dns = append(dns, dnsIP.String())
		}
	}
	return dns, nil
}

// ValidateIPAddress validates if the given value is a correctly formatted
// IP address, and returns the value in normalized form. Leading and trailing
// whitespace is allowed, but it does not allow IPv6 addresses surrounded by
// square brackets ("[::1]"). Refer to [net.ParseIP] for accepted formats.
func ValidateIPAddress(val string) (string, error) {
	if ip := net.ParseIP(strings.TrimSpace(val)); ip != nil {
		return ip.String(), nil
	}
	return "", fmt.Errorf("ip address is not correctly formatted: %q", val)
}
