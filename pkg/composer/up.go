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
	"os"

	"github.com/compose-spec/compose-go/v2/types"

	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/v2/pkg/reflectutil"
)

type UpOptions struct {
	AbortOnContainerExit bool
	Detach               bool
	NoBuild              bool
	NoColor              bool
	NoLogPrefix          bool
	ForceBuild           bool
	IPFS                 bool
	QuietPull            bool
	RemoveOrphans        bool
	ForceRecreate        bool
	NoRecreate           bool
	Scale                map[string]int // map of service name to replicas
	Pull                 string
}

func (opts UpOptions) recreateStrategy() string {
	switch {
	case opts.ForceRecreate:
		return RecreateForce
	case opts.NoRecreate:
		return RecreateNever
	default:
		return RecreateDiverged
	}
}

func (c *Composer) Up(ctx context.Context, uo UpOptions, services []string) error {
	for shortName := range c.project.Networks {
		if err := c.upNetwork(ctx, shortName); err != nil {
			return err
		}
	}

	for shortName := range c.project.Volumes {
		if err := c.upVolume(ctx, shortName); err != nil {
			return err
		}
	}

	for shortName, secret := range c.project.Secrets {
		obj := types.FileObjectConfig(secret)
		if err := validateFileObjectConfig(obj, shortName, "service", c.project); err != nil {
			return err
		}
	}

	for shortName, config := range c.project.Configs {
		obj := types.FileObjectConfig(config)
		if err := validateFileObjectConfig(obj, shortName, "config", c.project); err != nil {
			return err
		}
	}

	var parsedServices []*serviceparser.Service
	// use WithServices to sort the services in dependency order
	if err := c.project.ForEachService(services, func(name string, svc *types.ServiceConfig) error {
		if replicas, ok := uo.Scale[svc.Name]; ok {
			if svc.Deploy == nil {
				svc.Deploy = &types.DeployConfig{}
			}
			svc.Deploy.Replicas = &replicas
		}
		ps, err := serviceparser.Parse(c.project, *svc)
		if err != nil {
			return err
		}
		parsedServices = append(parsedServices, ps)
		return nil
	}); err != nil {
		return err
	}

	// remove orphan containers before the service has be started
	// FYI: https://github.com/docker/compose/blob/v2.3.4/pkg/compose/create.go#L91-L112
	orphans, err := c.getOrphanContainers(ctx, parsedServices)
	if err != nil && uo.RemoveOrphans {
		return fmt.Errorf("error getting orphaned containers: %w", err)
	}
	if len(orphans) > 0 {
		if uo.RemoveOrphans {
			if err := c.removeContainers(ctx, orphans, RemoveOptions{Stop: true, Volumes: true}); err != nil {
				return fmt.Errorf("error removing orphaned containers: %w", err)
			}
		} else {
			log.G(ctx).Warnf("found %d orphaned containers: %v, you can run this command with the --remove-orphans flag to clean it up", len(orphans), orphans)
		}
	}

	return c.upServices(ctx, parsedServices, uo)
}

func validateFileObjectConfig(obj types.FileObjectConfig, shortName, objType string, project *types.Project) error {
	if unknown := reflectutil.UnknownNonEmptyFields(&obj, "Name", "External", "File"); len(unknown) > 0 {
		log.L.Warnf("Ignoring: %s %s: %+v", objType, shortName, unknown)
	}

	if obj.File == "" {
		return fmt.Errorf("%s %q: lacks file path", objType, shortName)
	}
	fullPath := project.RelativePath(obj.File)
	if _, err := os.Stat(fullPath); err != nil {
		return fmt.Errorf("%s %q: failed to open file %q: %w", objType, shortName, fullPath, err)
	}
	return nil
}
