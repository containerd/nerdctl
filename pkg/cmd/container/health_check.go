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

package container

import (
	"context"
	"fmt"
	"time"

	containerd "github.com/containerd/containerd/v2/client"

	"github.com/containerd/nerdctl/v2/pkg/healthcheck"
	"github.com/containerd/nerdctl/v2/pkg/labels"
)

// HealthCheck executes the health check command for a container
func HealthCheck(ctx context.Context, client *containerd.Client, container containerd.Container) error {
	// verify container status and get task
	task, err := isContainerRunning(ctx, container)
	if err != nil {
		return err
	}

	// Check if container has health check configured
	info, err := container.Info(ctx)
	if err != nil {
		return fmt.Errorf("failed to get container info: %w", err)
	}
	hcConfigJSON, ok := info.Labels[labels.HealthCheck]
	if !ok {
		return fmt.Errorf("container has no health check configured")
	}

	// Parse health check configuration from labels
	var hcConfig *healthcheck.Healthcheck
	hcConfig, err = healthcheck.HealthCheckFromJSON(hcConfigJSON)
	if err != nil {
		return fmt.Errorf("invalid health check configuration: %w", err)
	}
	if hcConfig.Test == nil {
		return fmt.Errorf("health check configuration has no test")
	}

	// Populate defaults
	hcConfig.Interval = timeoutWithDefault(hcConfig.Interval, healthcheck.DefaultProbeInterval)
	hcConfig.Timeout = timeoutWithDefault(hcConfig.Timeout, healthcheck.DefaultProbeTimeout)
	hcConfig.StartPeriod = timeoutWithDefault(hcConfig.StartPeriod, healthcheck.DefaultStartPeriod)
	hcConfig.StartInterval = timeoutWithDefault(hcConfig.StartInterval, healthcheck.DefaultStartInterval)
	if hcConfig.Retries == 0 {
		hcConfig.Retries = healthcheck.DefaultProbeRetries
	}

	// Execute the health check
	return healthcheck.ExecuteHealthCheck(ctx, task, container, hcConfig)
}

func isContainerRunning(ctx context.Context, container containerd.Container) (containerd.Task, error) {
	// Get container task to check status
	task, err := container.Task(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get container task: %w", err)
	}

	// Check if container is running
	status, err := task.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get container status: %w", err)
	}
	if status.Status != containerd.Running {
		return nil, fmt.Errorf("container is not running (status: %s)", status.Status)
	}

	return task, nil
}

// If configuredValue is zero, use defaultValue instead.
func timeoutWithDefault(configuredValue time.Duration, defaultValue time.Duration) time.Duration {
	if configuredValue == 0 {
		return defaultValue
	}
	return configuredValue
}
