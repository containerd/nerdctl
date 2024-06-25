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
	"context"
	"fmt"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/netutil"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
)

func Prune(ctx context.Context, client *containerd.Client, options types.NetworkPruneOptions) error {
	e, err := netutil.NewCNIEnv(options.GOptions.CNIPath, options.GOptions.CNINetConfPath, netutil.WithNamespace(options.GOptions.Namespace))
	if err != nil {
		return err
	}

	usedNetworks, err := netutil.UsedNetworks(ctx, client)
	if err != nil {
		return err
	}

	networkConfigs, err := e.NetworkList()
	if err != nil {
		return err
	}

	var removedNetworks []string // nolint: prealloc
	for _, net := range networkConfigs {
		if strutil.InStringSlice(options.NetworkDriversToKeep, net.Name) {
			continue
		}
		if net.NerdctlID == nil || net.File == "" {
			continue
		}
		if _, ok := usedNetworks[net.Name]; ok {
			continue
		}
		if err := e.RemoveNetwork(net); err != nil {
			log.G(ctx).WithError(err).Errorf("failed to remove network %s", net.Name)
			continue
		}
		removedNetworks = append(removedNetworks, net.Name)
	}

	if len(removedNetworks) > 0 {
		fmt.Fprintln(options.Stdout, "Deleted Networks:")
		for _, name := range removedNetworks {
			fmt.Fprintln(options.Stdout, name)
		}
		fmt.Fprintln(options.Stdout, "")
	}
	return nil
}
