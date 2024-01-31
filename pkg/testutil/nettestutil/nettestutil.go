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

package nettestutil

import (
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/containerd/containerd/errdefs"
)

func HTTPGet(urlStr string, attempts int, insecure bool) (*http.Response, error) {
	var (
		resp *http.Response
		err  error
	)
	if attempts < 1 {
		return nil, errdefs.ErrInvalidArgument
	}
	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: insecure,
			},
		},
	}
	for i := 0; i < attempts; i++ {
		resp, err = client.Get(urlStr)
		if err == nil {
			return resp, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, fmt.Errorf("error after %d attempts: %w", attempts, err)
}

func NonLoopbackIPv4() (net.IP, error) {
	// no need to use [rootlessutil.WithDetachedNetNSIfAny] here
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			continue
		}
		ipv4 := ip.To4()
		if ipv4 == nil {
			continue
		}
		if ipv4.IsLoopback() {
			continue
		}
		return ipv4, nil
	}
	return nil, fmt.Errorf("non-loopback IPv4 address not found, attempted=%+v: %w", addrs, errdefs.ErrNotFound)
}

func GenerateMACAddress() (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	// make sure byte 0 (broadcast) of the first byte is not set
	// and byte 1 (local) is set
	buf[0] = buf[0]&254 | 2
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", buf[0], buf[1], buf[2], buf[3], buf[4], buf[5]), nil
}
