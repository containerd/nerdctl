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
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/config"
	"github.com/containerd/nerdctl/v2/pkg/defaults"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
)

// CreateTimer sets up the transient systemd timer and service for healthchecks.
func CreateTimer(ctx context.Context, container containerd.Container, cfg *config.Config) error {
	hc := extractHealthcheck(ctx, container)
	if hc == nil {
		return nil
	}
	if shouldSkipHealthCheckSystemd(hc, cfg) {
		return nil
	}

	containerID := container.ID()
	log.G(ctx).Debugf("Creating healthcheck timer unit: %s", containerID)

	cmdOpts := []string{}
	if path := os.Getenv("PATH"); path != "" {
		cmdOpts = append(cmdOpts, "--setenv=PATH="+path)
	}

	// Always use health-interval for timer frequency
	cmdOpts = append(cmdOpts, "--unit", containerID, "--on-unit-inactive="+hc.Interval.String(), "--timer-property=AccuracySec=1s")

	// Get the full path to the current nerdctl binary
	nerdctlPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine nerdctl executable path: %v", err)
	}

	cmdOpts = append(
		cmdOpts, nerdctlPath,
		"--namespace", cfg.Namespace,
		"--address", cfg.Address,
		"--data-root", cfg.DataRoot,
		"--cni-path", cfg.CNIPath,
		"--cni-netconfpath", cfg.CNINetConfPath,
		"--cgroup-manager", cfg.CgroupManager,
		"--host-gateway-ip", cfg.HostGatewayIP,
	)

	// Add boolean flags
	if cfg.InsecureRegistry {
		cmdOpts = append(cmdOpts, "--insecure-registry")
	}
	if cfg.Experimental {
		cmdOpts = append(cmdOpts, "--experimental")
	}
	if cfg.DebugFull {
		cmdOpts = append(cmdOpts, "--debug-full")
	}

	// Add array flags
	for _, dir := range cfg.HostsDir {
		cmdOpts = append(cmdOpts, "--hosts-dir", dir)
	}
	for _, dir := range cfg.CDISpecDirs {
		cmdOpts = append(cmdOpts, "--cdi-spec-dirs", dir)
	}

	// Add userns-remap if set
	if cfg.UsernsRemap != "" {
		cmdOpts = append(cmdOpts, "--userns-remap", cfg.UsernsRemap)
	}

	cmdOpts = append(cmdOpts, "container", "healthcheck", containerID)

	if log.G(ctx).Logger.IsLevelEnabled(log.DebugLevel) {
		cmdOpts = append(cmdOpts, "--debug")
	}

	if log.G(ctx).Logger.IsLevelEnabled(log.DebugLevel) {
		cmdOpts = append(cmdOpts, "--debug")
	}

	log.G(ctx).Debugf("creating healthcheck timer with: systemd-run %s", strings.Join(cmdOpts, " "))
	run := exec.Command("systemd-run", cmdOpts...)
	if out, err := run.CombinedOutput(); err != nil {
		return fmt.Errorf("systemd-run failed: %w\noutput: %s", err, strings.TrimSpace(string(out)))
	}

	return nil
}

// StartTimer starts the healthcheck timer unit.
func StartTimer(ctx context.Context, container containerd.Container, cfg *config.Config) error {
	hc := extractHealthcheck(ctx, container)
	if hc == nil {
		return nil
	}
	if shouldSkipHealthCheckSystemd(hc, cfg) {
		return nil
	}

	containerID := container.ID()
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
	unit := containerID + ".service"
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

	timer := containerID + ".timer"
	service := containerID + ".service"

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
func shouldSkipHealthCheckSystemd(hc *Healthcheck, cfg *config.Config) bool {
	// Don't proceed if systemd is unavailable or disabled
	if !defaults.IsSystemdAvailable() || cfg.DisableHCSystemd || rootlessutil.IsRootless() {
		return true
	}

	// Don't proceed if health check is nil, empty or explicitly NONE.
	if hc == nil || len(hc.Test) == 0 || hc.Test[0] == "NONE" {
		return true
	}
	return false
}
