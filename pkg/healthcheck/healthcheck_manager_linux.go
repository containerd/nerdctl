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

	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/sirupsen/logrus"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/defaults"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
)

// CreateTimer sets up the transient systemd timer and service for healthchecks.
func CreateTimer(ctx context.Context, container containerd.Container) error {
	hc := extractHealthcheck(ctx, container)
	if hc == nil {
		return nil
	}
	if shouldSkipHealthCheckSystemd(hc) {
		return nil
	}

	containerID := container.ID()
	hcName := hcUnitName(containerID, true)
	logrus.Debugf("Creating healthcheck timer unit: %s", hcName)

	cmd := []string{}
	if rootlessutil.IsRootless() {
		cmd = append(cmd, "--user")
	}
	if path := os.Getenv("PATH"); path != "" {
		cmd = append(cmd, "--setenv=PATH="+path)
	}

	// Always use health-interval for timer frequency
	cmd = append(cmd, "--unit", hcName, "--on-unit-inactive="+hc.Interval.String(), "--timer-property=AccuracySec=1s")

	cmd = append(cmd, "nerdctl", "container", "healthcheck", containerID)
	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		cmd = append(cmd, "--debug")
	}

	conn, err := dbus.NewSystemConnectionContext(context.Background())
	if err != nil {
		return fmt.Errorf("systemd DBUS connect error: %w", err)
	}
	defer conn.Close()

	logrus.Debugf("creating healthcheck timer with: systemd-run %s", strings.Join(cmd, " "))
	run := exec.Command("systemd-run", cmd...)
	if out, err := run.CombinedOutput(); err != nil {
		return fmt.Errorf("systemd-run failed: %w\noutput: %s", err, strings.TrimSpace(string(out)))
	}

	return nil
}

// StartTimer starts the healthcheck timer unit.
// TODO if we persist hcName to container state, pass that to this function.
func StartTimer(ctx context.Context, container containerd.Container) error {
	hc := extractHealthcheck(ctx, container)
	if hc == nil {
		return nil
	}
	if shouldSkipHealthCheckSystemd(hc) {
		return nil
	}

	hcName := hcUnitName(container.ID(), true)
	conn, err := dbus.NewSystemConnectionContext(context.Background())
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
	if shouldSkipHealthCheckSystemd(hc) {
		return nil
	}

	return RemoveTransientHealthCheckFilesByID(ctx, container.ID())
}

// RemoveTransientHealthCheckFilesByID stops and cleans up the transient timer and service using just the container ID.
func RemoveTransientHealthCheckFilesByID(ctx context.Context, containerID string) error {
	// Don't proceed if systemd is unavailable or disabled
	if !defaults.IsSystemdAvailable() || os.Getenv("DISABLE_HC_SYSTEMD") == "true" {
		return nil
	}

	logrus.Debugf("Removing healthcheck timer unit: %s", containerID)

	conn, err := dbus.NewSystemConnectionContext(context.Background())
	if err != nil {
		return fmt.Errorf("systemd DBUS connect error: %w", err)
	}
	defer conn.Close()

	unitName := hcUnitName(containerID, true)
	timer := unitName + ".timer"
	service := unitName + ".service"

	// Stop timer
	tChan := make(chan string)
	if _, err := conn.StopUnitContext(context.Background(), timer, "ignore-dependencies", tChan); err == nil {
		if msg := <-tChan; msg != "done" {
			logrus.Warnf("timer stop message: %s", msg)
		}
	}

	// Stop service
	sChan := make(chan string)
	if _, err := conn.StopUnitContext(context.Background(), service, "ignore-dependencies", sChan); err == nil {
		if msg := <-sChan; msg != "done" {
			logrus.Warnf("service stop message: %s", msg)
		}
	}

	// Reset failed units
	_ = conn.ResetFailedUnitContext(context.Background(), service)
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
	if !defaults.IsSystemdAvailable() || os.Getenv("DISABLE_HC_SYSTEMD") == "true" {
		return true
	}

	// Don't proceed if health check is nil, empty, explicitly NONE or interval is 0.
	if hc == nil || len(hc.Test) == 0 || hc.Test[0] == "NONE" || hc.Interval == 0 {
		return true
	}
	return false
}
