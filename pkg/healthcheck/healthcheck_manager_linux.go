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
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/defaults"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
)

// ShouldCreateTimer determines if a healthcheck timer should be created for the container.
func ShouldCreateTimer(ctx context.Context, container containerd.Container) bool {
	hc := extractHealthcheck(ctx, container)
	if hc == nil {
		return false
	}
	return !shouldSkipHealthCheckSystemd(hc)
}

// ShouldStartTimer determines if a healthcheck timer should be started for the container.
func ShouldStartTimer(ctx context.Context, container containerd.Container) bool {
	hc := extractHealthcheck(ctx, container)
	if hc == nil {
		return false
	}
	return !shouldSkipHealthCheckSystemd(hc)
}

// CreateTimer sets up the transient systemd timer and service for healthchecks.
// This function assumes the caller has already validated that a timer should be created.
func CreateTimer(ctx context.Context, container containerd.Container) error {
	hc := extractHealthcheck(ctx, container)
	// Note: We still need to extract healthcheck here for the actual timer creation logic
	// but we assume the caller has already validated it exists and is valid

	containerID := container.ID()
	hcName := hcUnitName(containerID, true)
	log.G(ctx).Debugf("Creating healthcheck timer unit: %s", hcName)

	cmd := []string{}
	if path := os.Getenv("PATH"); path != "" {
		cmd = append(cmd, "--setenv=PATH="+path)
	}

	// Always use health-interval for timer frequency
	cmd = append(cmd, "--unit", hcName, "--on-unit-inactive="+hc.Interval.String(), "--timer-property=AccuracySec=1s")

	cmd = append(cmd, "nerdctl", "container", "healthcheck", containerID)
	if log.G(ctx).Logger.IsLevelEnabled(log.DebugLevel) {
		cmd = append(cmd, "--debug")
	}

	log.G(ctx).Debugf("creating healthcheck timer with: systemd-run %s", strings.Join(cmd, " "))
	run := exec.Command("systemd-run", cmd...)
	if out, err := run.CombinedOutput(); err != nil {
		return fmt.Errorf("systemd-run failed: %w\noutput: %s", err, strings.TrimSpace(string(out)))
	}

	return nil
}

// StartTimer starts the healthcheck timer unit.
// This function assumes the caller has already validated that a timer should be started.
func StartTimer(ctx context.Context, container containerd.Container) error {
	hcName := hcUnitName(container.ID(), true)
	var conn *dbus.Conn
	var err error
	if rootlessutil.IsRootless() {
		conn, err = dbus.NewUserConnectionContext(ctx)
	} else {
		conn, err = dbus.NewSystemConnectionContext(ctx)
	}
	if err != nil {
		return fmt.Errorf("systemd DBUS connect error: %w", err)
	}
	defer conn.Close()

	startChan := make(chan string)
	unit := hcName + ".service"
	if _, err := conn.RestartUnitContext(context.Background(), unit, "fail", startChan); err != nil {
		return err
	}
	if msg := <-startChan; msg != "done" {
		return fmt.Errorf("unexpected systemd restart result: %s", msg)
	}
	return nil
}

// RemoveTransientHealthCheckFiles stops and cleans up the transient timer and service.
func RemoveTransientHealthCheckFiles(ctx context.Context, container containerd.Container) error {
	hc := extractHealthcheck(ctx, container)
	if hc == nil {
		return nil
	}

	return ForceRemoveTransientHealthCheckFiles(ctx, container.ID())
}

// ForceRemoveTransientHealthCheckFiles forcefully stops and cleans up the transient timer and service
// using just the container ID. This function is non-blocking and uses timeouts to prevent hanging
// on systemd operations. It logs errors as warnings but continues cleanup attempts.
func ForceRemoveTransientHealthCheckFiles(ctx context.Context, containerID string) error {
	log.G(ctx).Debugf("Force removing healthcheck timer unit: %s", containerID)

	// Create a timeout context for systemd operations
	timeoutCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	unitName := hcUnitName(containerID, true)
	timer := unitName + ".timer"
	service := unitName + ".service"

	// Channel to collect any critical errors (though we'll continue cleanup regardless)
	errChan := make(chan error, 3)

	// Goroutine for DBUS connection and cleanup operations
	go func() {
		defer close(errChan)

		var conn *dbus.Conn
		var err error
		if rootlessutil.IsRootless() {
			conn, err = dbus.NewUserConnectionContext(ctx)
		} else {
			conn, err = dbus.NewSystemConnectionContext(ctx)
		}
		if err != nil {
			log.G(ctx).Warnf("systemd DBUS connect error during force cleanup: %v", err)
			errChan <- fmt.Errorf("systemd DBUS connect error: %w", err)
			return
		}
		defer conn.Close()

		// Stop timer with timeout
		go func() {
			select {
			case <-timeoutCtx.Done():
				log.G(ctx).Warnf("timeout stopping timer %s during force cleanup", timer)
				return
			default:
				tChan := make(chan string, 1)
				if _, err := conn.StopUnitContext(timeoutCtx, timer, "ignore-dependencies", tChan); err == nil {
					select {
					case msg := <-tChan:
						if msg != "done" {
							log.G(ctx).Warnf("timer stop message during force cleanup: %s", msg)
						}
					case <-timeoutCtx.Done():
						log.G(ctx).Warnf("timeout waiting for timer stop confirmation: %s", timer)
					}
				} else {
					log.G(ctx).Warnf("failed to stop timer %s during force cleanup: %v", timer, err)
				}
			}
		}()

		// Stop service with timeout
		go func() {
			select {
			case <-timeoutCtx.Done():
				log.G(ctx).Warnf("timeout stopping service %s during force cleanup", service)
				return
			default:
				sChan := make(chan string, 1)
				if _, err := conn.StopUnitContext(timeoutCtx, service, "ignore-dependencies", sChan); err == nil {
					select {
					case msg := <-sChan:
						if msg != "done" {
							log.G(ctx).Warnf("service stop message during force cleanup: %s", msg)
						}
					case <-timeoutCtx.Done():
						log.G(ctx).Warnf("timeout waiting for service stop confirmation: %s", service)
					}
				} else {
					log.G(ctx).Warnf("failed to stop service %s during force cleanup: %v", service, err)
				}
			}
		}()

		// Reset failed units (best effort, non-blocking)
		go func() {
			select {
			case <-timeoutCtx.Done():
				log.G(ctx).Warnf("timeout resetting failed unit %s during force cleanup", service)
				return
			default:
				if err := conn.ResetFailedUnitContext(timeoutCtx, service); err != nil {
					log.G(ctx).Warnf("failed to reset failed unit %s during force cleanup: %v", service, err)
				}
			}
		}()

		// Wait a short time for operations to complete, but don't block indefinitely
		select {
		case <-time.After(3 * time.Second):
			log.G(ctx).Debugf("force cleanup operations completed for container %s", containerID)
		case <-timeoutCtx.Done():
			log.G(ctx).Warnf("force cleanup timed out for container %s", containerID)
		}
	}()

	// Wait for the cleanup goroutine to finish or timeout
	select {
	case err := <-errChan:
		if err != nil {
			log.G(ctx).Warnf("force cleanup encountered errors but continuing: %v", err)
		}
	case <-timeoutCtx.Done():
		log.G(ctx).Warnf("force cleanup timed out for container %s, but cleanup may continue in background", containerID)
	}

	// Always return nil - this function should never block the caller
	// even if systemd operations fail or timeout
	log.G(ctx).Debugf("force cleanup completed (non-blocking) for container %s", containerID)
	return nil
}

// hcUnitName returns a systemd unit name for a container healthcheck.
func hcUnitName(containerID string, bare bool) string {
	unit := containerID
	if !bare {
		unit += fmt.Sprintf("-%x", rand.Int())
	}
	return unit
}

func extractHealthcheck(ctx context.Context, container containerd.Container) *Healthcheck {
	l, err := container.Labels(ctx)
	if err != nil {
		log.G(ctx).WithError(err).Debugf("could not get labels for container %s", container.ID())
		return nil
	}
	hcStr, ok := l[labels.HealthCheck]
	if !ok || hcStr == "" {
		return nil
	}
	hc, err := HealthCheckFromJSON(hcStr)
	if err != nil {
		log.G(ctx).WithError(err).Debugf("invalid healthcheck config on container %s", container.ID())
		return nil
	}
	return hc
}

// shouldSkipHealthCheckSystemd determines if healthcheck timers should be skipped.
func shouldSkipHealthCheckSystemd(hc *Healthcheck) bool {
	// Don't proceed if systemd is unavailable or disabled
	if !defaults.IsSystemdAvailable() || os.Getenv("DISABLE_HC_SYSTEMD") == "true" || rootlessutil.IsRootless() {
		return true
	}

	// Don't proceed if health check is nil, empty, explicitly NONE or interval is 0.
	if hc == nil || len(hc.Test) == 0 || hc.Test[0] == "NONE" || hc.Interval == 0 {
		return true
	}
	return false
}
