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

package composer

import (
	"context"
	"fmt"

	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/reflectutil"
)

func (c *Composer) upNetwork(ctx context.Context, shortName string) error {
	net, ok := c.project.Networks[shortName]
	if !ok {
		return fmt.Errorf("invalid network name %q", shortName)
	}
	if net.External {
		// NOP
		return nil
	}

	if unknown := reflectutil.UnknownNonEmptyFields(&net, "Name", "Ipam", "Driver", "DriverOpts"); len(unknown) > 0 {
		log.G(ctx).Warnf("Ignoring: network %s: %+v", shortName, unknown)
	}

	// shortName is like "default", fullName is like "compose-wordpress_default"
	fullName := net.Name
	netExists, err := c.NetworkExists(fullName)
	if err != nil {
		return err
	} else if !netExists {
		log.G(ctx).Infof("Creating network %s", fullName)
		//add metadata labels to network https://github.com/compose-spec/compose-spec/blob/master/spec.md#labels-1
		createArgs := []string{
			fmt.Sprintf("--label=%s=%s", labels.ComposeProject, c.project.Name),
			fmt.Sprintf("--label=%s=%s", labels.ComposeNetwork, shortName),
		}

		if net.Driver != "" {
			createArgs = append(createArgs, fmt.Sprintf("--driver=%s", net.Driver))
		}

		if net.DriverOpts != nil {
			for k, v := range net.DriverOpts {
				createArgs = append(createArgs, fmt.Sprintf("--opt=%s=%s", k, v))
			}
		}

		if net.Ipam.Config != nil {
			if len(net.Ipam.Config) > 1 {
				log.G(ctx).Warnf("Ignoring: network %s: imam.config %+v", shortName, net.Ipam.Config[1:])
			}

			ipamConfig := net.Ipam.Config[0]
			if unknown := reflectutil.UnknownNonEmptyFields(ipamConfig, "Subnet", "Gateway", "IPRange"); len(unknown) > 0 {
				log.G(ctx).Warnf("Ignoring: network %s: ipam.config[0]: %+v", shortName, unknown)
			}
			if ipamConfig.Subnet != "" {
				createArgs = append(createArgs, fmt.Sprintf("--subnet=%s", ipamConfig.Subnet))
			}
			if ipamConfig.Gateway != "" {
				createArgs = append(createArgs, fmt.Sprintf("--gateway=%s", ipamConfig.Gateway))
			}
			if ipamConfig.IPRange != "" {
				createArgs = append(createArgs, fmt.Sprintf("--ip-range=%s", ipamConfig.IPRange))
			}
		}

		createArgs = append(createArgs, fullName)

		if c.DebugPrintFull {
			log.G(ctx).Debugf("Creating network args: %s", createArgs)
		}

		if err := c.runNerdctlCmd(ctx, append([]string{"network", "create"}, createArgs...)...); err != nil {
			return err
		}
	}
	return nil
}
