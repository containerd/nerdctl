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
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/runtime/restart"
	"github.com/containerd/nerdctl/pkg/portutil"
	"github.com/opencontainers/runtime-spec/specs-go"
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
