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
	"slices"

	"github.com/compose-spec/compose-go/v2/types"

	containerd "github.com/containerd/containerd/v2/client"

	"github.com/containerd/nerdctl/v2/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/v2/pkg/labels"
)

// ScaleOptions specifies the options for Scale.
type ScaleOptions struct {
	// Scale maps a service name to its number of replicas.
	Scale map[string]int
	// NoDeps does not start the services the scaled services depend on.
	NoDeps bool
}

// Scale sets the number of replicas of services, like `docker compose scale`.
// Missing replicas are created and started in the background, and replicas
// above the requested number are stopped and removed.
func (c *Composer) Scale(ctx context.Context, options ScaleOptions) error {
	services := make([]string, 0, len(options.Scale))
	for name, replicas := range options.Scale {
		if replicas < 0 {
			return fmt.Errorf("invalid number of replicas %d for service %q", replicas, name)
		}
		services = append(services, name)
	}
	slices.Sort(services)
	if _, err := c.project.GetServices(services...); err != nil {
		return err
	}

	uo := UpOptions{
		Detach: true,
		Scale:  options.Scale,
		NoDeps: options.NoDeps,
	}
	if err := c.Up(ctx, uo, services); err != nil {
		return err
	}

	for _, name := range services {
		extra, err := c.extraReplicas(ctx, name, options.Scale[name])
		if err != nil {
			return err
		}
		if len(extra) == 0 {
			continue
		}
		if err := c.removeContainers(ctx, extra, RemoveOptions{Stop: true}); err != nil {
			return err
		}
	}
	return nil
}

// extraReplicas returns the containers of the service that are not among its
// first `replicas` replicas.
func (c *Composer) extraReplicas(ctx context.Context, name string, replicas int) ([]containerd.Container, error) {
	svc, err := c.project.GetService(name)
	if err != nil {
		return nil, err
	}
	// copy the deploy config, so that the project is not modified
	deploy := types.DeployConfig{}
	if svc.Deploy != nil {
		deploy = *svc.Deploy
	}
	deploy.Replicas = &replicas
	svc.Deploy = &deploy

	ps, err := serviceparser.Parse(c.project, svc)
	if err != nil {
		return nil, err
	}
	wanted := make(map[string]struct{}, len(ps.Containers))
	for _, container := range ps.Containers {
		wanted[container.Name] = struct{}{}
	}

	containers, err := c.Containers(ctx, name)
	if err != nil {
		return nil, err
	}
	var extra []containerd.Container
	for _, container := range containers {
		containerLabels, err := container.Labels(ctx)
		if err != nil {
			return nil, err
		}
		if _, ok := wanted[containerLabels[labels.Name]]; !ok {
			extra = append(extra, container)
		}
	}
	return extra, nil
}
