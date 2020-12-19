/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.
   Copyright (C) Docker/Moby authors.

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

// Package dockercompat mimics `docker inspect` objects.
package dockercompat

import (
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/AkihiroSuda/nerdctl/pkg/inspecttypes/native"
	"github.com/AkihiroSuda/nerdctl/pkg/labels"
	"github.com/containerd/containerd"
	"github.com/opencontainers/runtime-spec/specs-go"
)

// Container mimics a `docker container inspect` object.
// From https://github.com/moby/moby/blob/v20.10.1/api/types/types.go#L340-L374
type Container struct {
	ID             string `json:"Id"`
	Created        string
	Path           string
	Args           []string
	State          *ContainerState
	Image          string
	ResolvConfPath string
	// TODO: HostnamePath   string
	// TODO: HostsPath      string
	LogPath string
	// Unimplemented: Node            *ContainerNode `json:",omitempty"` // Node is only propagated by Docker Swarm standalone API
	Name string
	// TODO: RestartCount int
	Driver   string
	Platform string
	// TODO: MountLabel      string
	// TODO: ProcessLabel    string
	AppArmorProfile string
	// TODO: ExecIDs         []string
	// TODO: HostConfig      *container.HostConfig
	// TODO: GraphDriver     GraphDriverData
	// TODO: SizeRw     *int64 `json:",omitempty"`
	// TODO: SizeRootFs *int64 `json:",omitempty"`

	// TODO: Mounts          []MountPoint
	// TODO: Config          *container.Config
	// TODO: NetworkSettings *NetworkSettings
}

// ContainerState is from https://github.com/moby/moby/blob/v20.10.1/api/types/types.go#L313-L326
type ContainerState struct {
	Status  string // String representation of the container state. Can be one of "created", "running", "paused", "restarting", "removing", "exited", or "dead"
	Running bool
	Paused  bool
	// TODO:	Restarting bool
	// TODO: OOMKilled  bool
	// TODO:	Dead       bool
	Pid      int
	ExitCode int
	// TODO: Error      string
	// TODO: StartedAt  string
	FinishedAt string
	// TODO: Health     *Health `json:",omitempty"`
}

// ContainerFromNative instantiates a Docker-compatible Container from containerd-native Container.
func ContainerFromNative(n *native.Container) (*Container, error) {
	c := &Container{
		ID:       n.ID,
		Created:  n.CreatedAt.Format(time.RFC3339Nano),
		Image:    n.Image,
		Name:     n.Labels[labels.Name],
		Driver:   n.Snapshotter,
		Platform: runtime.GOOS,
	}
	if sp, ok := n.Spec.(*specs.Spec); ok {
		if p := sp.Process; p != nil {
			if len(p.Args) > 0 {
				c.Path = p.Args[0]
				if len(p.Args) > 1 {
					c.Args = p.Args[1:]
				}
			}
			c.AppArmorProfile = p.ApparmorProfile
		}
	}
	if nerdctlStateDir := n.Labels[labels.StateDir]; nerdctlStateDir != "" {
		c.ResolvConfPath = filepath.Join(nerdctlStateDir, "resolv.conf")
		if _, err := os.Stat(c.ResolvConfPath); err != nil {
			c.ResolvConfPath = ""
		}
		c.LogPath = filepath.Join(nerdctlStateDir, n.ID+"-json.log")
		if _, err := os.Stat(c.LogPath); err != nil {
			c.LogPath = ""
		}
	}
	if n.Process != nil {
		c.State = &ContainerState{
			Status:     string(n.Process.Status.Status),
			Running:    n.Process.Status.Status == containerd.Running,
			Paused:     n.Process.Status.Status == containerd.Paused,
			Pid:        n.Process.Pid,
			ExitCode:   int(n.Process.Status.ExitStatus),
			FinishedAt: n.Process.Status.ExitTime.Format(time.RFC3339Nano),
		}
	}
	return c, nil
}
