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

package containerutil

import (
	"context"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/runtime/restart"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/portutil"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/moby/sys/signal"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

// PrintHostPort writes to `writer` the public (HostIP:HostPort) of a given `containerPort/protocol` in a container.
// if `containerPort < 0`, it writes all public ports of the container.
func PrintHostPort(ctx context.Context, writer io.Writer, container containerd.Container, containerPort int, proto string) error {
	l, err := container.Labels(ctx)
	if err != nil {
		return err
	}
	ports, err := portutil.ParsePortsLabel(l)
	if err != nil {
		return err
	}

	if containerPort < 0 {
		for _, p := range ports {
			fmt.Fprintf(writer, "%d/%s -> %s:%d\n", p.ContainerPort, p.Protocol, p.HostIP, p.HostPort)
		}
		return nil
	}

	for _, p := range ports {
		if p.ContainerPort == int32(containerPort) && strings.ToLower(p.Protocol) == proto {
			fmt.Fprintf(writer, "%s:%d\n", p.HostIP, p.HostPort)
			return nil
		}
	}
	return fmt.Errorf("no public port %d/%s published for %q", containerPort, proto, container.ID())
}

// ContainerStatus returns the container's status from its task.
func ContainerStatus(ctx context.Context, c containerd.Container) (containerd.Status, error) {
	// Just in case, there is something wrong in server.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	task, err := c.Task(ctx, nil)
	if err != nil {
		return containerd.Status{}, err
	}

	return task.Status(ctx)
}

// ContainerNetNSPath returns the netns path of a container.
func ContainerNetNSPath(ctx context.Context, c containerd.Container) (string, error) {
	task, err := c.Task(ctx, nil)
	if err != nil {
		return "", err
	}
	status, err := task.Status(ctx)
	if err != nil {
		return "", err
	}
	if status.Status != containerd.Running {
		return "", fmt.Errorf("invalid target container: %s, should be running", c.ID())
	}
	return fmt.Sprintf("/proc/%d/ns/net", task.Pid()), nil
}

// UpdateExplicitlyStoppedLabel updates the "containerd.io/restart.explicitly-stopped"
// label of the container according to the value of explicitlyStopped.
func UpdateExplicitlyStoppedLabel(ctx context.Context, container containerd.Container, explicitlyStopped bool) error {
	opt := containerd.WithAdditionalContainerLabels(map[string]string{
		restart.ExplicitlyStoppedLabel: strconv.FormatBool(explicitlyStopped),
	})
	return container.Update(ctx, containerd.UpdateContainerOpts(opt))
}

// WithBindMountHostProcfs replaces procfs mount with rbind.
// Required for --pid=host on rootless.
//
// https://github.com/moby/moby/pull/41893/files
// https://github.com/containers/podman/blob/v3.0.0-rc1/pkg/specgen/generate/oci.go#L248-L257
func WithBindMountHostProcfs(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
	for i, m := range s.Mounts {
		if path.Clean(m.Destination) == "/proc" {
			newM := specs.Mount{
				Destination: "/proc",
				Type:        "bind",
				Source:      "/proc",
				Options:     []string{"rbind", "nosuid", "noexec", "nodev"},
			}
			s.Mounts[i] = newM
		}
	}

	// Remove ReadonlyPaths for /proc/*
	newROP := s.Linux.ReadonlyPaths[:0]
	for _, x := range s.Linux.ReadonlyPaths {
		x = path.Clean(x)
		if !strings.HasPrefix(x, "/proc/") {
			newROP = append(newROP, x)
		}
	}
	s.Linux.ReadonlyPaths = newROP
	return nil
}

// GenerateSharingPIDOpts returns the oci.SpecOpts that shares the host linux namespace from `targetCon`
// If `targetCon` doesn't have a `PIDNamespace`, a new one is generated from its `Pid`.
func GenerateSharingPIDOpts(ctx context.Context, targetCon containerd.Container) ([]oci.SpecOpts, error) {
	opts := make([]oci.SpecOpts, 0)

	task, err := targetCon.Task(ctx, nil)
	if err != nil {
		return nil, err
	}
	status, err := task.Status(ctx)
	if err != nil {
		return nil, err
	}

	if status.Status != containerd.Running {
		return nil, fmt.Errorf("shared container is not running")
	}

	spec, err := targetCon.Spec(ctx)
	if err != nil {
		return nil, err
	}

	isHost := true
	for _, n := range spec.Linux.Namespaces {
		if n.Type == specs.PIDNamespace {
			isHost = false
		}
	}
	if isHost {
		opts = append(opts, oci.WithHostNamespace(specs.PIDNamespace))
		if rootlessutil.IsRootless() {
			opts = append(opts, WithBindMountHostProcfs)
		}
	} else {
		ns := specs.LinuxNamespace{
			Type: specs.PIDNamespace,
			Path: fmt.Sprintf("/proc/%d/ns/pid", task.Pid()),
		}
		opts = append(opts, oci.WithLinuxNamespace(ns))
	}
	return opts, nil
}

// Stop stops `container` by sending SIGTERM. If the container is not stopped after `timeout`, it sends a SIGKILL.
func Stop(ctx context.Context, container containerd.Container, timeout *time.Duration) error {
	if err := UpdateExplicitlyStoppedLabel(ctx, container, true); err != nil {
		return err
	}

	l, err := container.Labels(ctx)
	if err != nil {
		return err
	}

	if timeout == nil {
		t, ok := l[labels.StopTimout]
		if !ok {
			// Default is 10 seconds.
			t = "10"
		}
		td, err := time.ParseDuration(t + "s")
		if err != nil {
			return err
		}
		timeout = &td
	}

	task, err := container.Task(ctx, cio.Load)
	if err != nil {
		return err
	}

	status, err := task.Status(ctx)
	if err != nil {
		return err
	}

	paused := false

	switch status.Status {
	case containerd.Created, containerd.Stopped:
		return nil
	case containerd.Paused, containerd.Pausing:
		paused = true
	default:
	}

	// NOTE: ctx is main context so that it's ok to use for task.Wait().
	exitCh, err := task.Wait(ctx)
	if err != nil {
		return err
	}

	if *timeout > 0 {
		sig, err := signal.ParseSignal("SIGTERM")
		if err != nil {
			return err
		}
		if stopSignal, ok := l[containerd.StopSignalLabel]; ok {
			sig, err = signal.ParseSignal(stopSignal)
			if err != nil {
				return err
			}
		}

		if err := task.Kill(ctx, sig); err != nil {
			return err
		}

		// signal will be sent once resume is finished
		if paused {
			if err := task.Resume(ctx); err != nil {
				logrus.Warnf("Cannot unpause container %s: %s", container.ID(), err)
			} else {
				// no need to do it again when send sigkill signal
				paused = false
			}
		}

		sigtermCtx, sigtermCtxCancel := context.WithTimeout(ctx, *timeout)
		defer sigtermCtxCancel()

		err = waitContainerStop(sigtermCtx, exitCh, container.ID())
		if err == nil {
			return nil
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	sig, err := signal.ParseSignal("SIGKILL")
	if err != nil {
		return err
	}

	if err := task.Kill(ctx, sig); err != nil {
		return err
	}

	// signal will be sent once resume is finished
	if paused {
		if err := task.Resume(ctx); err != nil {
			logrus.Warnf("Cannot unpause container %s: %s", container.ID(), err)
		}
	}
	return waitContainerStop(ctx, exitCh, container.ID())
}

func waitContainerStop(ctx context.Context, exitCh <-chan containerd.ExitStatus, id string) error {
	select {
	case <-ctx.Done():
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("wait container %v: %w", id, err)
		}
		return nil
	case status := <-exitCh:
		return status.Error()
	}
}
