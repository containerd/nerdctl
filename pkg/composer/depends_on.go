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
	"sort"
	"time"

	"github.com/compose-spec/compose-go/v2/types"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/v2/pkg/healthcheck"
	"github.com/containerd/nerdctl/v2/pkg/labels"
)

// dependencyPollInterval is how often the health of a dependency is checked.
// A variable so that tests can shorten it.
var dependencyPollInterval = time.Second

// waitForDependencies blocks until the dependencies of a service that declare
// a `service_healthy` or `service_completed_successfully` condition are
// satisfied, like `docker compose up` does before it starts the service.
// Dependencies with the default `service_started` condition need no waiting,
// since the services are started in dependency order.
func (c *Composer) waitForDependencies(ctx context.Context, ps *serviceparser.Service) error {
	names := make([]string, 0, len(ps.Unparsed.DependsOn))
	for name := range ps.Unparsed.DependsOn {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		var err error
		switch ps.Unparsed.DependsOn[name].Condition {
		case types.ServiceConditionHealthy:
			log.G(ctx).Infof("Waiting for service %s to be healthy", name)
			err = c.waitServiceHealthy(ctx, name)
		case types.ServiceConditionCompletedSuccessfully:
			log.G(ctx).Infof("Waiting for service %s to complete successfully", name)
			err = c.waitServiceCompleted(ctx, name)
		}
		if err != nil {
			return fmt.Errorf("dependency %s of service %s failed to start: %w", name, ps.Unparsed.Name, err)
		}
	}
	return nil
}

func (c *Composer) waitServiceHealthy(ctx context.Context, service string) error {
	containers, err := c.Containers(ctx, service)
	if err != nil {
		return err
	}
	if len(containers) == 0 {
		return fmt.Errorf("service %s has no containers", service)
	}
	for _, container := range containers {
		if err := waitContainerHealthy(ctx, container); err != nil {
			return err
		}
	}
	return nil
}

func waitContainerHealthy(ctx context.Context, container containerd.Container) error {
	ticker := time.NewTicker(dependencyPollInterval)
	defer ticker.Stop()
	for {
		lbs, err := container.Labels(ctx)
		if err != nil {
			return err
		}
		if lbs[labels.HealthCheck] == "" {
			return fmt.Errorf("container %s has no healthcheck configured", container.ID())
		}
		if state := lbs[labels.HealthState]; state != "" {
			hs, err := healthcheck.HealthStateFromJSON(state)
			if err != nil {
				return fmt.Errorf("failed to parse the health state of container %s: %w", container.ID(), err)
			}
			switch hs.Status {
			case healthcheck.Healthy:
				return nil
			case healthcheck.Unhealthy:
				return fmt.Errorf("container %s is unhealthy", container.ID())
			}
		}
		if task, err := container.Task(ctx, nil); err == nil {
			if status, err := task.Status(ctx); err == nil && status.Status == containerd.Stopped {
				return fmt.Errorf("container %s exited with status code %d before it became healthy", container.ID(), status.ExitStatus)
			}
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (c *Composer) waitServiceCompleted(ctx context.Context, service string) error {
	containers, err := c.Containers(ctx, service)
	if err != nil {
		return err
	}
	if len(containers) == 0 {
		return fmt.Errorf("service %s has no containers", service)
	}
	for _, container := range containers {
		task, err := container.Task(ctx, nil)
		if err != nil {
			if errdefs.IsNotFound(err) {
				return fmt.Errorf("container %s was not started", container.ID())
			}
			return err
		}
		statusC, err := task.Wait(ctx)
		if err != nil {
			return err
		}
		select {
		case status := <-statusC:
			code, _, err := status.Result()
			if err != nil {
				return err
			}
			if code != 0 {
				return fmt.Errorf("container %s exited with status code %d", container.ID(), code)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
