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

package network

import (
	"fmt"
	"io"
	"sort"

	"github.com/containerd/errdefs"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/netutil"
)

func Create(options types.NetworkCreateOptions, stdout io.Writer) error {
	// A nil IPv4 defaults to enabled.
	ipv4 := options.IPv4 == nil || *options.IPv4
	// At least one address family must be enabled, matching docker which
	// rejects a network with both IPv4 and IPv6 turned off.
	if !ipv4 && !options.IPv6 {
		return fmt.Errorf("IPv4 or IPv6 must be enabled")
	}
	if !ipv4 && len(options.Subnets) == 0 {
		// IPv6-only needs a concrete IPv6 subnet: unlike docker, nerdctl does
		// not auto-allocate one, and the empty-subnet default below would pick
		// an IPv4 range, contradicting the disabled IPv4.
		return fmt.Errorf("IPv6-only network requires an IPv6 subnet, specify --subnet manually")
	}
	if len(options.Subnets) == 0 {
		// Docker matches each aux-address to a subnet that contains it, so
		// without any subnet there is nothing to match. Surface the same
		// "no matching subnet for aux-address <ip>" error Docker returns.
		aux, err := netutil.ParseAuxAddresses(options.AuxAddresses)
		if err != nil {
			return err
		}
		if len(aux) > 0 {
			// Report a stable IP: map iteration order is random, so sort first.
			ips := make([]string, 0, len(aux))
			for _, ip := range aux {
				ips = append(ips, ip)
			}
			sort.Strings(ips)
			return fmt.Errorf("no matching subnet for aux-address %s", ips[0])
		}
		if len(options.Gateway) > 0 || len(options.IPRange) > 0 {
			return fmt.Errorf("cannot set gateway or ip-range without subnet, specify --subnet manually")
		}
		options.Subnets = []string{""}
	}

	e, err := netutil.NewCNIEnv(options.GOptions.CNIPath, options.GOptions.CNINetConfPath, netutil.WithNamespace(options.GOptions.Namespace))
	if err != nil {
		return err
	}
	net, err := e.CreateNetwork(options)
	if err != nil {
		if errdefs.IsAlreadyExists(err) {
			return fmt.Errorf("network with name %s already exists", options.Name)
		}
		return err
	}
	_, err = fmt.Fprintln(stdout, *net.NerdctlID)
	return err
}
