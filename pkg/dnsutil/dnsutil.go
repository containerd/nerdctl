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

	"github.com/containerd/nerdctl/pkg/rootlessutil"
)

func GetSlirp4netnsDns() ([]string, error) {
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
		for _, dnsIp := range info.NetworkDriver.DNS {
			dns = append(dns, dnsIp.String())
		}
	}
	return dns, nil
}
