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
	"github.com/containerd/nerdctl/v2/pkg/strutil"
)

type DownOptions struct {
	RemoveVolumes bool
	RemoveOrphans bool
}

func (c *Composer) Down(ctx context.Context, downOptions DownOptions) error {
	serviceNames, err := c.ServiceNames()
	if err != nil {
		return err
	}
	// reverse dependency order
	for _, svc := range strutil.ReverseStrSlice(serviceNames) {
		containers, err := c.Containers(ctx, svc)
		if err != nil {
			return err
		}
		// use default Options to stop service containers.
		if err := c.stopContainers(ctx, containers, StopOptions{}); err != nil {
			return err
		}
		if err := c.removeContainers(ctx, containers, RemoveOptions{Stop: true, Volumes: downOptions.RemoveVolumes}); err != nil {
			return err
		}
	}

	// remove orphan containers
	parsedServices, err := c.Services(ctx)
	if err != nil {
		return err
	}
	orphans, err := c.getOrphanContainers(ctx, parsedServices)
	if err != nil && downOptions.RemoveOrphans {
		return fmt.Errorf("error getting orphaned containers: %s", err)
	}
	if len(orphans) > 0 {
		if downOptions.RemoveOrphans {
			if err := c.removeContainers(ctx, orphans, RemoveOptions{Stop: true, Volumes: downOptions.RemoveVolumes}); err != nil {
				return fmt.Errorf("error removeing orphaned containers: %s", err)
			}
		} else {
			log.G(ctx).Warnf("found %d orphaned containers: %v, you can run this command with the --remove-orphans flag to clean it up", len(orphans), orphans)
		}
	}

	for shortName := range c.project.Networks {
		if err := c.downNetwork(ctx, shortName); err != nil {
			return err
		}
	}

	if downOptions.RemoveVolumes {
		for shortName := range c.project.Volumes {
			if err := c.downVolume(ctx, shortName); err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *Composer) downNetwork(ctx context.Context, shortName string) error {
	net, ok := c.project.Networks[shortName]
	if !ok {
		return fmt.Errorf("invalid network name %q", shortName)
	}
	if net.External {
		// NOP
		return nil
	}
	// shortName is like "default", fullName is like "compose-wordpress_default"
	fullName := net.Name
	netExists, err := c.NetworkExists(fullName)
	if err != nil {
		return err
	} else if netExists {
		netUsed, err := c.NetworkInUse(ctx, fullName)
		if err != nil {
			return err
		}
		if netUsed {
			return fmt.Errorf("network %s is in use", fullName)
		}

		log.G(ctx).Infof("Removing network %s", fullName)
		if err := c.runNerdctlCmd(ctx, "network", "rm", fullName); err != nil {
			log.G(ctx).Warn(err)
		}
	}
	return nil
}

func (c *Composer) downVolume(ctx context.Context, shortName string) error {
	vol, ok := c.project.Volumes[shortName]
	if !ok {
		return fmt.Errorf("invalid volume name %q", shortName)
	}
	if vol.External {
		// NOP
		return nil
	}
	// shortName is like "db_data", fullName is like "compose-wordpress_db_data"
	fullName := vol.Name
	volExists, err := c.VolumeExists(fullName)
	if err != nil {
		return err
	} else if volExists {
		log.G(ctx).Infof("Removing volume %s", fullName)
		if err := c.runNerdctlCmd(ctx, "volume", "rm", "-f", fullName); err != nil {
			log.G(ctx).Warn(err)
		}
	}
	return nil
}
