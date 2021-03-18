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

/*
   Portions from https://github.com/moby/moby/blob/v20.10.1/api/types/types.go
   Copyright (C) Docker/Moby authors.
   Licensed under the Apache License, Version 2.0
*/

// Package dockercompat mimics `docker inspect` objects.
package dockercompat

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
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
	NetworkSettings *NetworkSettings
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

type NetworkSettings struct {
	DefaultNetworkSettings
	Networks map[string]*NetworkEndpointSettings
}

// DefaultNetworkSettings is from https://github.com/moby/moby/blob/v20.10.1/api/types/types.go#L405-L414
type DefaultNetworkSettings struct {
	// TODO EndpointID          string // EndpointID uniquely represents a service endpoint in a Sandbox
	// TODO Gateway             string // Gateway holds the gateway address for the network
	GlobalIPv6Address   string // GlobalIPv6Address holds network's global IPv6 address
	GlobalIPv6PrefixLen int    // GlobalIPv6PrefixLen represents mask length of network's global IPv6 address
	IPAddress           string // IPAddress holds the IPv4 address for the network
	IPPrefixLen         int    // IPPrefixLen represents mask length of network's IPv4 address
	// TODO IPv6Gateway         string // IPv6Gateway holds gateway address specific for IPv6
	MacAddress string // MacAddress holds the MAC address for the network
}

// NetworkEndpointSettings is from https://github.com/moby/moby/blob/v20.10.1/api/types/network/network.go#L49-L65
type NetworkEndpointSettings struct {
	// Configurations
	// TODO IPAMConfig *EndpointIPAMConfig
	// TODO Links      []string
	// TODO Aliases    []string
	// Operational data
	// TODO NetworkID           string
	// TODO EndpointID          string
	// TODO Gateway             string
	IPAddress   string
	IPPrefixLen int
	// TODO IPv6Gateway         string
	GlobalIPv6Address   string
	GlobalIPv6PrefixLen int
	MacAddress          string
	// TODO DriverOpts          map[string]string
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
			Status:     statusFromNative(n.Process.Status.Status),
			Running:    n.Process.Status.Status == containerd.Running,
			Paused:     n.Process.Status.Status == containerd.Paused,
			Pid:        n.Process.Pid,
			ExitCode:   int(n.Process.Status.ExitStatus),
			FinishedAt: n.Process.Status.ExitTime.Format(time.RFC3339Nano),
		}
		c.NetworkSettings = networkSettingsFromNative(n.Process.NetNS)
	}
	return c, nil
}

func statusFromNative(x containerd.ProcessStatus) string {
	switch x {
	case containerd.Stopped:
		return "exited"
	default:
		return string(x)
	}
}

func networkSettingsFromNative(n *native.NetNS) *NetworkSettings {
	if n == nil {
		return nil
	}
	res := &NetworkSettings{
		Networks: make(map[string]*NetworkEndpointSettings),
	}
	var primary *NetworkEndpointSettings
	for _, x := range n.Interfaces {
		if x.Interface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if x.Interface.Flags&net.FlagUp == 0 {
			continue
		}
		nes := &NetworkEndpointSettings{}
		nes.MacAddress = x.HardwareAddr
		for _, a := range x.Addrs {
			ip, ipnet, err := net.ParseCIDR(a)
			if err != nil {
				logrus.WithError(err).WithField("name", x.Name).Warnf("failed to parse %q", a)
				continue
			}
			if ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			ones, _ := ipnet.Mask.Size()
			if ip4 := ip.To4(); ip4 != nil {
				nes.IPAddress = ip4.String()
				nes.IPPrefixLen = ones
			} else if ip16 := ip.To16(); ip16 != nil {
				nes.GlobalIPv6Address = ip16.String()
				nes.GlobalIPv6PrefixLen = ones
			}
		}
		// TODO: set CNI name when possible
		fakeDockerNetworkName := fmt.Sprintf("unknown-%s", x.Name)
		res.Networks[fakeDockerNetworkName] = nes
		if x.Index == n.PrimaryInterface {
			primary = nes
		}
	}
	if primary != nil {
		res.DefaultNetworkSettings.MacAddress = primary.MacAddress
		res.DefaultNetworkSettings.IPAddress = primary.IPAddress
		res.DefaultNetworkSettings.IPPrefixLen = primary.IPPrefixLen
		res.DefaultNetworkSettings.GlobalIPv6Address = primary.GlobalIPv6Address
		res.DefaultNetworkSettings.GlobalIPv6PrefixLen = primary.GlobalIPv6PrefixLen
	}
	return res
}
