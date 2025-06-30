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

package healthcheck

import (
	"context"
	"fmt"
	"strings"
	"syscall"
	"time"

	"github.com/opencontainers/runtime-spec/specs-go"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/idgen"
)

// ExecuteHealthCheck executes the health check command for a container
func ExecuteHealthCheck(ctx context.Context, task containerd.Task, container containerd.Container, hc *Healthcheck) error {
	// Prepare process spec for health check command
	processSpec, err := prepareProcessSpec(ctx, container, hc)
	if err != nil {
		return err
	}
	if processSpec == nil {
		return nil
	}

	startTime := time.Now()
	result, err := probeHealthCheck(ctx, task, hc, processSpec)
	if err != nil {
		_ = updateHealthStatus(ctx, container, hc, &HealthcheckResult{
			Start:    startTime,
			End:      time.Now(),
			ExitCode: -1,
			Output:   err.Error(),
		})
		return fmt.Errorf("health check probe failed: %w", err)
	}

	// Success case, update health status
	result.Start = startTime
	if err := updateHealthStatus(ctx, container, hc, result); err != nil {
		return fmt.Errorf("failed to update health status: %w", err)
	}
	return nil
}

// probeHealthCheck executes the health check command inside the container context
func probeHealthCheck(ctx context.Context, task containerd.Task, hc *Healthcheck, processSpec *specs.Process) (*HealthcheckResult, error) {
	execID := "health-check-" + idgen.TruncateID(idgen.GenerateID())
	outputBuf := NewResizableBuffer(MaxOutputLen)

	process, err := task.Exec(ctx, execID, processSpec, cio.NewCreator(
		cio.WithStreams(nil, outputBuf, outputBuf),
	))
	if err != nil {
		log.G(ctx).Debugf("failed to exec health check: %v", err)
		return nil, fmt.Errorf("exec error: %w", err)
	}

	if err := process.Start(ctx); err != nil {
		log.G(ctx).Debugf("failed to start health check: %v", err)
		return nil, fmt.Errorf("start error: %w", err)
	}

	exitStatusC, err := process.Wait(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for health check: %w", err)
	}

	select {
	case <-time.After(hc.Timeout):
		_ = process.Kill(ctx, syscall.SIGKILL)
		<-exitStatusC
		process.IO().Wait()
		process.IO().Close()
		msg := fmt.Sprintf("Health check exceeded timeout (%v)", hc.Timeout)
		if out := outputBuf.String(); len(out) > 0 {
			msg = fmt.Sprintf("Health check exceeded timeout (%v): %s", hc.Timeout, out)
		}

		log.G(ctx).Debugf("health check timed out: %s", msg)

		return &HealthcheckResult{
			ExitCode: -1,
			Output:   msg,
			End:      time.Now(),
		}, nil

	case exitStatus := <-exitStatusC:
		process.IO().Wait()
		process.IO().Close()
		code, _, _ := exitStatus.Result()
		return &HealthcheckResult{
			ExitCode: int(code),
			Output:   outputBuf.String(),
			End:      time.Now(),
		}, nil
	}
}

// updateHealthStatus updates the health status based on the health check result
func updateHealthStatus(ctx context.Context, container containerd.Container, hcConfig *Healthcheck, hcResult *HealthcheckResult) error {
	// Get current health state from labels
	currentHealth, err := readHealthStateFromLabels(ctx, container)
	if err != nil {
		return fmt.Errorf("failed to read health state from labels: %w", err)
	}
	if currentHealth == nil {
		currentHealth = &HealthState{
			Status:        Starting,
			FailingStreak: 0,
		}
	}

	// Check if still within start period
	startPeriod := hcConfig.StartPeriod
	info, err := container.Info(ctx)
	if err != nil {
		return fmt.Errorf("failed to get container info: %w", err)
	}
	containerCreated := info.CreatedAt
	stillInStartPeriod := hcResult.Start.Sub(containerCreated) < startPeriod

	// Update health status based on exit code
	if hcResult.ExitCode == 0 {
		currentHealth.Status = Healthy
		currentHealth.FailingStreak = 0
	} else if !stillInStartPeriod {
		currentHealth.FailingStreak++
		if currentHealth.FailingStreak >= hcConfig.Retries {
			currentHealth.Status = Unhealthy
		}
	}

	// Write updated health state back to labels
	if err := writeHealthStateToLabels(ctx, container, currentHealth); err != nil {
		return fmt.Errorf("failed to write health state to labels: %w", err)
	}

	// Store the latest health check result in the log file
	if err := writeHealthLog(ctx, container, hcResult); err != nil {
		return fmt.Errorf("failed to write health log: %w", err)
	}
	return nil
}

// prepareProcessSpec prepares the process spec for health check execution
func prepareProcessSpec(ctx context.Context, container containerd.Container, hcConfig *Healthcheck) (*specs.Process, error) {
	hcCommand := hcConfig.Test

	var args []string
	switch hcCommand[0] {
	case TestNone, CmdNone:
		log.G(ctx).Debug("health check is set to NONE, skipping execution")
		return nil, nil
	case Cmd:
		args = hcCommand[1:]
	case CmdShell:
		if len(hcCommand) < 2 || strings.TrimSpace(hcCommand[1]) == "" {
			return nil, fmt.Errorf("no health check command specified")
		}
		args = []string{"/bin/sh", "-c", strings.Join(hcCommand[1:], " ")}
	default:
		args = hcCommand
	}

	if len(args) < 1 || args[0] == "" {
		return nil, fmt.Errorf("no health check command specified")
	}

	// Get container spec for environment and working directory
	spec, err := container.Spec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get container spec: %w", err)
	}
	processSpec := &specs.Process{
		Args: args,
		Env:  spec.Process.Env,
		User: spec.Process.User,
		Cwd:  spec.Process.Cwd,
	}

	return processSpec, nil
}
