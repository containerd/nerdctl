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
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/containerd/containerd"
	gocni "github.com/containerd/go-cni"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/docker/go-connections/nat"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

// Image mimics a `docker image inspect` object.
// From https://github.com/moby/moby/blob/v20.10.1/api/types/types.go#L340-L374
type Image struct {
	ID          string `json:"Id"`
	RepoTags    []string
	RepoDigests []string
	// TODO: Parent      string
	Comment string
	Created string
	// TODO: Container   string
	// TODO: ContainerConfig *container.Config
	// TODO: DockerVersion string
	Author       string
	Config       *Config
	Architecture string
	// TODO: Variant       string `json:",omitempty"`
	Os string
	// TODO: OsVersion     string `json:",omitempty"`
	// TODO: Size          int64
	// TODO: VirtualSize   int64
	// TODO: GraphDriver     GraphDriverData
	RootFS   RootFS
	Metadata ImageMetadata
}

type RootFS struct {
	Type      string
	Layers    []string `json:",omitempty"`
	BaseLayer string   `json:",omitempty"`
}

type ImageMetadata struct {
	LastTagTime time.Time `json:",omitempty"`
}

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
	HostnamePath   string
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
	Mounts MountPoint
	// TODO: Config          *container.Config
	NetworkSettings *NetworkSettings
}
type MountPoint []struct {
	Destination string   `json:"destination"`
	Source      string   `json:"source"`
	Options     []string `json:"options"`
}

//config is from https://github.com/moby/moby/blob/8dbd90ec00daa26dc45d7da2431c965dec99e8b4/api/types/container/config.go#L37-L69
type Config struct {
	// TODO: Hostname     string      // Hostname
	// TODO: Domainname   string      // Domainname
	User string // User that will run the command(s) inside the container, also support user:group
	// TODO: AttachStdin  bool        // Attach the standard input, makes possible user interaction
	// TODO: AttachStdout bool        // Attach the standard output
	// TODO: AttachStderr bool        // Attach the standard error
	ExposedPorts nat.PortSet `json:",omitempty"` // List of exposed ports
	// TODO: Tty          bool        // Attach standard streams to a tty, including stdin if it is not closed.
	// TODO: OpenStdin    bool        // Open stdin
	// TODO: StdinOnce    bool        // If true, close stdin after the 1 attached client disconnects.
	Env []string // List of environment variable to set in the container
	Cmd []string // Command to run when starting the container
	// TODO Healthcheck     *HealthConfig       `json:",omitempty"` // Healthcheck describes how to check the container is healthy
	// TODO: ArgsEscaped     bool                `json:",omitempty"` // True if command is already escaped (meaning treat as a command line) (Windows specific).
	// TODO: Image           string              // Name of the image as it was passed by the operator (e.g. could be symbolic)
	Volumes    map[string]struct{} // List of volumes (mounts) used for the container
	WorkingDir string              // Current directory (PWD) in the command will be launched
	Entrypoint []string            // Entrypoint to run when starting the container
	// TODO: NetworkDisabled bool                `json:",omitempty"` // Is network disabled
	// TODO: MacAddress      string              `json:",omitempty"` // Mac Address of the container
	// TODO: OnBuild         []string            // ONBUILD metadata that were defined on the image Dockerfile
	Labels map[string]string // List of labels set to this container
	// TODO: StopSignal      string              `json:",omitempty"` // Signal to stop a container
	// TODO: StopTimeout     *int                `json:",omitempty"` // Timeout (in seconds) to stop a container
	// TODO: Shell           []string            `json:",omitempty"` // Shell for shell-form of RUN, CMD, ENTRYPOINT
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
	Ports *nat.PortMap `json:",omitempty"`
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
	var mount *Specs
	err := json.Unmarshal(n.Container.Spec.Value, &mount)
	if err != nil {
		return nil, err
	}
	c := &Container{
		ID:       n.ID,
		Created:  n.CreatedAt.Format(time.RFC3339Nano),
		Image:    n.Image,
		Name:     n.Labels[labels.Name],
		Driver:   n.Snapshotter,
		Mounts:   mount.Mounts,
		Platform: runtime.GOOS, // for Docker compatibility, this Platform string does NOT contain arch like "/amd64"
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
		c.HostnamePath = filepath.Join(nerdctlStateDir, "hostname")
		if _, err := os.Stat(c.HostnamePath); err != nil {
			c.HostnamePath = ""
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
		nSettings, err := networkSettingsFromNative(n.Process.NetNS, n.Spec.(*specs.Spec))
		if err != nil {
			return nil, err
		}
		c.NetworkSettings = nSettings
	}
	return c, nil
}

func ImageFromNative(n *native.Image) (*Image, error) {
	i := &Image{}

	imgoci := n.ImageConfig

	i.RootFS.Type = imgoci.RootFS.Type
	diffIDs := imgoci.RootFS.DiffIDs
	for _, d := range diffIDs {
		i.RootFS.Layers = append(i.RootFS.Layers, d.String())
	}
	if len(imgoci.History) > 0 {
		i.Comment = imgoci.History[len(imgoci.History)-1].Comment
		i.Created = imgoci.History[len(imgoci.History)-1].Created.String()
		i.Author = imgoci.History[len(imgoci.History)-1].Author
	}
	i.Architecture = imgoci.Architecture
	i.Os = imgoci.OS

	portSet := make(nat.PortSet)
	for k := range imgoci.Config.ExposedPorts {
		portSet[nat.Port(k)] = struct{}{}
	}

	i.Config = &Config{
		Cmd:          imgoci.Config.Cmd,
		Volumes:      imgoci.Config.Volumes,
		Env:          imgoci.Config.Env,
		User:         imgoci.Config.User,
		WorkingDir:   imgoci.Config.WorkingDir,
		Entrypoint:   imgoci.Config.Entrypoint,
		Labels:       imgoci.Config.Labels,
		ExposedPorts: portSet,
	}

	i.ID = n.ImageConfigDesc.Digest.String() // Docker ID (digest of platform-specific config), not containerd ID (digest of multi-platform index or manifest)

	repository, tag := imgutil.ParseRepoTag(n.Image.Name)

	i.RepoTags = []string{fmt.Sprintf("%s:%s", repository, tag)}
	i.RepoDigests = []string{fmt.Sprintf("%s@%s", repository, n.Image.Target.Digest.String())}

	return i, nil
}
func statusFromNative(x containerd.ProcessStatus) string {
	switch x {
	case containerd.Stopped:
		return "exited"
	default:
		return string(x)
	}
}

func networkSettingsFromNative(n *native.NetNS, sp *specs.Spec) (*NetworkSettings, error) {
	if n == nil {
		return nil, nil
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

		if portsLabel, ok := sp.Annotations[labels.Ports]; ok {
			var ports []gocni.PortMapping
			err := json.Unmarshal([]byte(portsLabel), &ports)
			if err != nil {
				return nil, err
			}
			nports, err := convertToNatPort(ports)
			if err != nil {
				return nil, err
			}
			res.Ports = nports
		}
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
	return res, nil
}

func convertToNatPort(portMappings []gocni.PortMapping) (*nat.PortMap, error) {
	portMap := make(nat.PortMap)
	for _, portMapping := range portMappings {
		ports := []nat.PortBinding{}
		p := nat.PortBinding{
			HostIP:   portMapping.HostIP,
			HostPort: strconv.FormatInt(int64(portMapping.HostPort), 10),
		}
		newP, err := nat.NewPort(portMapping.Protocol, strconv.FormatInt(int64(portMapping.ContainerPort), 10))
		if err != nil {
			return nil, err
		}
		ports = append(ports, p)
		portMap[newP] = ports
	}
	return &portMap, nil
}

type IPAMConfig struct {
	Subnet  string `json:"Subnet,omitempty"`
	Gateway string `json:"Gateway,omitempty"`
	IPRange string `json:"IPRange,omitempty"`
}

type IPAM struct {
	// Driver is omitted
	Config []IPAMConfig `json:"Config,omitempty"`
}

// Network mimics a `docker network inspect` object.
// From https://github.com/moby/moby/blob/v20.10.7/api/types/types.go#L430-L448
type Network struct {
	Name   string            `json:"Name"`
	ID     string            `json:"Id,omitempty"` // optional in nerdctl
	IPAM   IPAM              `json:"IPAM,omitempty"`
	Labels map[string]string `json:"Labels"`
	// Scope, Driver, etc. are omitted
}

func NetworkFromNative(n *native.Network) (*Network, error) {
	var res Network

	nameResult := gjson.GetBytes(n.CNI, "name")
	if s, ok := nameResult.Value().(string); ok {
		res.Name = s
	}

	// flatten twice to get ipamRangesResult=[{ "subnet": "10.4.19.0/24", "gateway": "10.4.19.1" }]
	ipamRangesResult := gjson.GetBytes(n.CNI, "plugins.#.ipam.ranges|@flatten|@flatten")
	for _, f := range ipamRangesResult.Array() {
		m := f.Map()
		var cfg IPAMConfig
		if x, ok := m["subnet"]; ok {
			cfg.Subnet = x.String()
		}
		if x, ok := m["gateway"]; ok {
			cfg.Gateway = x.String()
		}
		if x, ok := m["ipRange"]; ok {
			cfg.IPRange = x.String()
		}
		res.IPAM.Config = append(res.IPAM.Config, cfg)
	}

	if n.NerdctlID != nil {
		res.ID = strconv.Itoa(*n.NerdctlID)
	}

	if n.NerdctlLabels != nil {
		res.Labels = *n.NerdctlLabels
	}

	return &res, nil
}

type Specs struct {
	OciVersion string `json:"ociVersion"`
	Process    struct {
		Terminal bool `json:"terminal"`
		User     struct {
			UID      int    `json:"uid"`
			Gid      int    `json:"gid"`
			Username string `json:"username"`
		} `json:"user"`
		Args []string `json:"args"`
		Env  []string `json:"env"`
		Cwd  string   `json:"cwd"`
	} `json:"process"`
	Root struct {
		Path string `json:"path"`
	} `json:"root"`
	Hostname string     `json:"hostname"`
	Mounts   MountPoint `json:"mounts,omitempty"`
	Hooks    struct {
		CreateRuntime []struct {
			Path string   `json:"path"`
			Args []string `json:"args"`
			Env  []string `json:"env"`
		} `json:"createRuntime"`
		Poststop []struct {
			Path string   `json:"path"`
			Args []string `json:"args"`
			Env  []string `json:"env"`
		} `json:"poststop"`
	} `json:"hooks"`
	Annotations struct {
		IoContainerdImageConfigStopSignal string `json:"io.containerd.image.config.stop-signal"`
		NerdctlExtraHosts                 string `json:"nerdctl/extraHosts"`
		NerdctlHostname                   string `json:"nerdctl/hostname"`
		NerdctlName                       string `json:"nerdctl/name"`
		NerdctlNamespace                  string `json:"nerdctl/namespace"`
		NerdctlNetworks                   string `json:"nerdctl/networks"`
		NerdctlPlatform                   string `json:"nerdctl/platform"`
		NerdctlStateDir                   string `json:"nerdctl/state-dir"`
	} `json:"annotations"`
	Windows struct {
		LayerFolders            interface{} `json:"layerFolders"`
		IgnoreFlushesDuringBoot bool        `json:"ignoreFlushesDuringBoot"`
		Network                 struct {
			AllowUnqualifiedDNSQuery bool `json:"allowUnqualifiedDNSQuery"`
		} `json:"network"`
	} `json:"windows"`
}
