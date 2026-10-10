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
	run, err := shouldRunProbe(ctx, container, hc)
	if err != nil {
		return err
	}
	if !run {
		log.G(ctx).Infof("skipping health check for %s: next probe is not due yet", container.ID())
		return nil
	}

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

// shouldRunProbe reports whether a probe is due on this tick. The timer ticks at the faster of
// --health-interval and --health-start-interval, so ticks that arrive too early are skipped.
func shouldRunProbe(ctx context.Context, container containerd.Container, hc *Healthcheck) (bool, error) {
	state, err := readHealthStateFromLabels(ctx, container)
	if err != nil {
		return false, fmt.Errorf("failed to read health state from labels: %w", err)
	}
	// First tick: nothing recorded yet.
	if state == nil {
		return true, nil
	}

	info, err := container.Info(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get container info: %w", err)
	}

	return shouldRunProbeAt(time.Now(), info.CreatedAt, state.LastProbeAt, state.InStartPeriod, hc), nil
}

// shouldRunProbeAt is the pure decision behind shouldRunProbe.
func shouldRunProbeAt(now, containerCreated, lastProbeAt time.Time, inStartPeriod bool, hc *Healthcheck) bool {
	if lastProbeAt.IsZero() {
		return true
	}

	target := hc.Interval
	if hc.StartPeriod > 0 && inStartPeriod {
		// Like Docker, if less than start-interval is left of the start period after the last
		// probe, the next probe is due when the start period ends.
		if left := containerCreated.Add(hc.StartPeriod).Sub(lastProbeAt); left > 0 {
			target = min(hc.StartInterval, left)
		}
	}
	return now.Sub(lastProbeAt) >= target
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
	defer func() {
		if _, err := process.Delete(ctx); err != nil {
			log.G(ctx).WithError(err).Debug("failed to delete exec process")
		}
	}()

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
		// Determine if we should start in the start period workflow
		hasStartPeriod := hcConfig.StartPeriod > 0
		currentHealth = &HealthState{
			Status:        Starting,
			FailingStreak: 0,
			InStartPeriod: hasStartPeriod,
		}
	}

	// Get container info for start period check
	info, err := container.Info(ctx)
	if err != nil {
		return fmt.Errorf("failed to get container info: %w", err)
	}
	containerCreated := info.CreatedAt

	// Check if we're in start period workflow
	inStartPeriodTime := hcResult.Start.Sub(containerCreated) < hcConfig.StartPeriod
	inStartPeriodState := currentHealth.InStartPeriod

	if inStartPeriodTime && inStartPeriodState {
		// Start Period Workflow
		if hcResult.ExitCode == 0 {
			// First healthy result transitions us out of start period
			currentHealth.Status = Healthy
			currentHealth.FailingStreak = 0
			currentHealth.InStartPeriod = false
		}
		// Ignore unhealthy results during start period
	} else {
		// Health Interval Workflow
		if hcResult.ExitCode == 0 {
			if currentHealth.Status != Healthy {
				currentHealth.Status = Healthy
				currentHealth.FailingStreak = 0
			}
		} else {
			currentHealth.FailingStreak++
			if currentHealth.FailingStreak >= hcConfig.Retries && currentHealth.Status != Unhealthy {
				currentHealth.Status = Unhealthy
			}
		}
	}

	// Record the probe time for shouldRunProbe.
	currentHealth.LastProbeAt = hcResult.Start
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
