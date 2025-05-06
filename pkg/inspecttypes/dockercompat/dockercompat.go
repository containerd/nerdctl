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
   NOTICE: https://github.com/moby/moby/blob/v20.10.1/NOTICE
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
	"strings"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/docker/go-units"
	"github.com/opencontainers/runtime-spec/specs-go"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/runtime/restart"
	"github.com/containerd/go-cni"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/imgutil"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/v2/pkg/ipcutil"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/ocihook/state"
)

// From https://github.com/moby/moby/blob/v26.1.2/api/types/types.go#L34-L140
type Image struct {
	ID            string `json:"Id"`
	RepoTags      []string
	RepoDigests   []string
	Parent        string
	Comment       string
	Created       string
	DockerVersion string
	Author        string
	Config        *Config
	Architecture  string
	Variant       string `json:",omitempty"`
	Os            string

	// TODO: OsVersion     string `json:",omitempty"`

	Size        int64 // Size is the unpacked size of the image
	VirtualSize int64 `json:"VirtualSize,omitempty"` // Deprecated

	// TODO: GraphDriver	GraphDriverData

	RootFS   RootFS
	Metadata ImageMetadata

	// Deprecated: TODO: Container   string
	// Deprecated: TODO: ContainerConfig *container.Config
}

// From: https://github.com/moby/moby/blob/v26.1.2/api/types/graph_driver_data.go
type GraphDriverData struct {
	Data map[string]string `json:"Data"`
	Name string            `json:"Name"`
}

type RootFS struct {
	Type      string
	Layers    []string `json:",omitempty"`
	BaseLayer string   `json:",omitempty"`
}

type ImageMetadata struct {
	LastTagTime time.Time `json:",omitempty"`
}

type loggerLogConfig struct {
	Driver  string            `json:"driver"`
	Opts    map[string]string `json:"opts,omitempty"`
	LogURI  string            `json:"-"`
	Address string            `json:"address"`
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
	HostsPath      string
	LogPath        string
	// Unimplemented: Node            *ContainerNode `json:",omitempty"` // Node is only propagated by Docker Swarm standalone API
	Name         string
	RestartCount int
	Driver       string
	Platform     string
	// TODO: MountLabel      string
	// TODO: ProcessLabel    string
	AppArmorProfile string
	// TODO: ExecIDs         []string
	HostConfig *HostConfig
	// TODO: GraphDriver     GraphDriverData
	SizeRw     *int64 `json:",omitempty"`
	SizeRootFs *int64 `json:",omitempty"`

	Mounts          []MountPoint
	Config          *Config
	NetworkSettings *NetworkSettings
}

// From https://github.com/moby/moby/blob/8dbd90ec00daa26dc45d7da2431c965dec99e8b4/api/types/container/host_config.go#L391
// HostConfig the non-portable Config structure of a container.
type HostConfig struct {
	// Binds           []string      // List of volume bindings for this container
	ContainerIDFile string          // File (path) where the containerId is written
	LogConfig       loggerLogConfig // Configuration of the logs for this container
	// NetworkMode     NetworkMode   // Network mode to use for the container
	PortBindings nat.PortMap // Port mapping between the exposed port (container) and the host
	// RestartPolicy   RestartPolicy // Restart policy to be used for the container
	// AutoRemove      bool          // Automatically remove container when it exits
	// VolumeDriver    string        // Name of the volume driver used to mount volumes
	// VolumesFrom     []string      // List of volumes to take from other container
	// CapAdd          strslice.StrSlice // List of kernel capabilities to add to the container
	// CapDrop         strslice.StrSlice // List of kernel capabilities to remove from the container

	CgroupnsMode string   // Cgroup namespace mode to use for the container
	DNS          []string `json:"Dns"`        // List of DNS server to lookup
	DNSOptions   []string `json:"DnsOptions"` // List of DNSOption to look for
	DNSSearch    []string `json:"DnsSearch"`  // List of DNSSearch to look for
	ExtraHosts   []string // List of extra hosts
	GroupAdd     []string // GroupAdd specifies additional groups to join
	IpcMode      string   `json:"IpcMode"` // IPC namespace to use for the container
	// Cgroup          CgroupSpec        // Cgroup to use for the container
	OomScoreAdj int    // specifies the tune container’s OOM preferences (-1000 to 1000, rootless: 100 to 1000)
	PidMode     string // PID namespace to use for the container
	// Privileged      bool              // Is the container in privileged mode
	// PublishAllPorts bool              // Should docker publish all exposed port for the container
	ReadonlyRootfs bool // Is the container root filesystem in read-only
	// SecurityOpt     []string          // List of string values to customize labels for MLS systems, such as SELinux.
	Tmpfs   map[string]string `json:"Tmpfs,omitempty"` // List of tmpfs (mounts) used for the container
	UTSMode string            // UTS namespace to use for the container
	// UsernsMode      UsernsMode        // The user namespace to use for the container
	ShmSize            int64             // Size of /dev/shm in bytes. The size must be greater than 0.
	Sysctls            map[string]string // List of Namespaced sysctls used for the container
	Runtime            string            // Runtime to use with this container
	CPUSetMems         string            `json:"CpusetMems"`         // CpusetMems 0-2, 0,1
	CPUSetCPUs         string            `json:"CpusetCpus"`         // CpusetCpus 0-2, 0,1
	CPUQuota           int64             `json:"CpuQuota"`           // CPU CFS (Completely Fair Scheduler) quota
	CPUShares          uint64            `json:"CpuShares"`          // CPU shares (relative weight vs. other containers)
	CPUPeriod          uint64            `json:"CpuPeriod"`          // Limits the CPU CFS (Completely Fair Scheduler) period
	CPURealtimePeriod  uint64            `json:"CpuRealtimePeriod"`  // Limits the CPU real-time period in microseconds
	CPURealtimeRuntime int64             `json:"CpuRealtimeRuntime"` // Limits the CPU real-time runtime in microseconds
	Memory             int64             // Memory limit (in bytes)
	MemorySwap         int64             // Total memory usage (memory + swap); set `-1` to enable unlimited swap
	OomKillDisable     bool              // specifies whether to disable OOM Killer
	Devices            []DeviceMapping   // List of devices to map inside the container
	LinuxBlkioSettings
}

// From https://github.com/moby/moby/blob/v20.10.1/api/types/types.go#L416-L427
// MountPoint represents a mount point configuration inside the container.
// This is used for reporting the mountpoints in use by a container.
type MountPoint struct {
	Type        string `json:",omitempty"`
	Name        string `json:",omitempty"`
	Source      string
	Destination string
	Driver      string `json:",omitempty"`
	Mode        string
	RW          bool
	Propagation string
}

// config is from https://github.com/moby/moby/blob/8dbd90ec00daa26dc45d7da2431c965dec99e8b4/api/types/container/config.go#L37-L69
type Config struct {
	Hostname    string `json:",omitempty"` // Hostname
	Domainname  string `json:",omitempty"` // Domainname
	User        string `json:",omitempty"` // User that will run the command(s) inside the container, also support user:group
	AttachStdin bool   // Attach the standard input, makes possible user interaction
	// TODO: AttachStdout bool        // Attach the standard output
	// TODO: AttachStderr bool        // Attach the standard error
	ExposedPorts nat.PortSet `json:",omitempty"` // List of exposed ports
	// TODO: Tty          bool        // Attach standard streams to a tty, including stdin if it is not closed.
	// TODO: OpenStdin    bool        // Open stdin
	// TODO: StdinOnce    bool        // If true, close stdin after the 1 attached client disconnects.
	Env []string `json:",omitempty"` // List of environment variable to set in the container
	Cmd []string `json:",omitempty"` // Command to run when starting the container
	// TODO Healthcheck     *HealthConfig       `json:",omitempty"` // Healthcheck describes how to check the container is healthy
	// TODO: ArgsEscaped     bool                `json:",omitempty"` // True if command is already escaped (meaning treat as a command line) (Windows specific).
	// TODO: Image           string              // Name of the image as it was passed by the operator (e.g. could be symbolic)
	Volumes    map[string]struct{} `json:",omitempty"` // List of volumes (mounts) used for the container
	WorkingDir string              `json:",omitempty"` // Current directory (PWD) in the command will be launched
	Entrypoint []string            `json:",omitempty"` // Entrypoint to run when starting the container
	// TODO: NetworkDisabled bool                `json:",omitempty"` // Is network disabled
	// TODO: MacAddress      string              `json:",omitempty"` // Mac Address of the container
	// TODO: OnBuild         []string            // ONBUILD metadata that were defined on the image Dockerfile
	Labels map[string]string `json:",omitempty"` // List of labels set to this container
	// TODO: StopSignal      string              `json:",omitempty"` // Signal to stop a container
	// TODO: StopTimeout     *int                `json:",omitempty"` // Timeout (in seconds) to stop a container
	// TODO: Shell           []string            `json:",omitempty"` // Shell for shell-form of RUN, CMD, ENTRYPOINT
}

// ContainerState is from https://github.com/moby/moby/blob/v20.10.1/api/types/types.go#L313-L326
type ContainerState struct {
	Status     string // String representation of the container state. Can be one of "created", "running", "paused", "restarting", "removing", "exited", or "dead"
	Running    bool
	Paused     bool
	Restarting bool
	// TODO: OOMKilled  bool
	// TODO:	Dead       bool
	Pid        int
	ExitCode   int
	Error      string
	StartedAt  string
	FinishedAt string
	// TODO: Health     *Health `json:",omitempty"`
}

type NetworkSettings struct {
	Ports *nat.PortMap
	DefaultNetworkSettings
	Networks map[string]*NetworkEndpointSettings
}

type DNSSettings struct {
	DNSServers           []string
	DNSResolvConfOptions []string
	DNSSearchDomains     []string
}

type HostConfigLabel struct {
	BlkioWeight uint16
	CidFile     string
	Devices     []DeviceMapping
}

type DeviceMapping struct {
	PathOnHost        string
	PathInContainer   string
	CgroupPermissions string
}

type CPUSettings struct {
	CPUSetCpus         string
	CPUSetMems         string
	CPUShares          uint64
	CPUQuota           int64
	CPUPeriod          uint64
	CPURealtimePeriod  uint64
	CPURealtimeRuntime int64
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

type LinuxBlkioSettings struct {
	BlkioWeight          uint16 // Block IO weight (relative weight vs. other containers)
	BlkioWeightDevice    []*specs.LinuxWeightDevice
	BlkioDeviceReadBps   []*specs.LinuxThrottleDevice
	BlkioDeviceWriteBps  []*specs.LinuxThrottleDevice
	BlkioDeviceReadIOps  []*specs.LinuxThrottleDevice
	BlkioDeviceWriteIOps []*specs.LinuxThrottleDevice
}

// ContainerFromNative instantiates a Docker-compatible Container from containerd-native Container.
func ContainerFromNative(n *native.Container) (*Container, error) {
	var hostname string
	c := &Container{
		ID:      n.ID,
		Created: n.CreatedAt.Format(time.RFC3339Nano),
		Image:   n.Image,
		Name:    n.Labels[labels.Name],
		Driver:  n.Snapshotter,
		// XXX is this always right? what if the container OS is NOT the same as the host OS?
		Platform: runtime.GOOS, // for Docker compatibility, this Platform string does NOT contain arch like "/amd64"
	}
	c.HostConfig = new(HostConfig)
	if n.Labels[restart.StatusLabel] == string(containerd.Running) {
		c.RestartCount, _ = strconv.Atoi(n.Labels[restart.CountLabel])
	}
	containerAnnotations := make(map[string]string)
	if sp, ok := n.Spec.(*specs.Spec); ok {
		containerAnnotations = sp.Annotations
		if p := sp.Process; p != nil {
			if len(p.Args) > 0 {
				c.Path = p.Args[0]
				if len(p.Args) > 1 {
					c.Args = p.Args[1:]
				}
			}
			c.AppArmorProfile = p.ApparmorProfile
		}
		c.Mounts = mountsFromNative(sp.Mounts)
		for _, mount := range c.Mounts {
			if mount.Destination == "/etc/resolv.conf" {
				c.ResolvConfPath = mount.Source
			} else if mount.Destination == "/etc/hostname" {
				c.HostnamePath = mount.Source
			} else if mount.Destination == "/etc/hosts" {
				c.HostsPath = mount.Source
			}
		}
		hostname = sp.Hostname
	}
	if nerdctlStateDir := n.Labels[labels.StateDir]; nerdctlStateDir != "" {
		resolvConfPath := filepath.Join(nerdctlStateDir, "resolv.conf")
		if _, err := os.Stat(resolvConfPath); err == nil {
			c.ResolvConfPath = resolvConfPath
		}
		hostnamePath := filepath.Join(nerdctlStateDir, "hostname")
		if _, err := os.Stat(hostnamePath); err == nil {
			c.HostnamePath = hostnamePath
		}
		c.LogPath = filepath.Join(nerdctlStateDir, n.ID+"-json.log")
		if _, err := os.Stat(c.LogPath); err != nil {
			c.LogPath = ""
		}
		hostsPath := filepath.Join(nerdctlStateDir, "hosts")
		if _, err := os.Stat(hostsPath); err == nil {
			c.HostsPath = hostsPath
		}
	}

	c.HostConfig.Tmpfs = make(map[string]string)
	if nerdctlMounts := n.Labels[labels.Mounts]; nerdctlMounts != "" {
		mounts, err := parseMounts(nerdctlMounts)
		if err != nil {
			return nil, err
		}
		c.Mounts = mounts
		for _, mount := range mounts {
			if mount.Type == "tmpfs" {
				c.HostConfig.Tmpfs[mount.Destination] = mount.Mode
			}
		}
	}

	if nedctlExtraHosts := n.Labels[labels.ExtraHosts]; nedctlExtraHosts != "" {
		c.HostConfig.ExtraHosts = parseExtraHosts(nedctlExtraHosts)
	}

	if nerdctlLoguri := n.Labels[labels.LogURI]; nerdctlLoguri != "" {
		c.HostConfig.LogConfig.LogURI = nerdctlLoguri
	}
	if logConfigJSON, ok := n.Labels[labels.LogConfig]; ok {
		var logConfig loggerLogConfig
		err := json.Unmarshal([]byte(logConfigJSON), &logConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal log config: %v", err)
		}

		// Assign the parsed LogConfig to c.HostConfig.LogConfig
		c.HostConfig.LogConfig = logConfig
	} else {
		// If LogConfig label is not present, set default values
		c.HostConfig.LogConfig = loggerLogConfig{
			Driver: "json-file",
			Opts:   make(map[string]string),
		}
	}

	hostConfigLabel, err := getHostConfigLabelFromNative(n.Labels)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch HostConfigLabel: %v", err)
	}

	c.HostConfig.BlkioWeight = hostConfigLabel.BlkioWeight
	c.HostConfig.ContainerIDFile = hostConfigLabel.CidFile

	groupAdd, err := groupAddFromNative(n.Spec.(*specs.Spec))
	if err != nil {
		return nil, fmt.Errorf("failed to groupAdd from native spec: %v", err)
	}

	c.HostConfig.GroupAdd = groupAdd
	c.HostConfig.ShmSize = 0

	if ipcMode := n.Labels[labels.IPC]; ipcMode != "" {
		ipc, err := ipcutil.DecodeIPCLabel(ipcMode)
		if err != nil {
			return nil, fmt.Errorf("failed to Decode IPC Label: %v", err)
		}
		c.HostConfig.IpcMode = string(ipc.Mode)
		if ipc.ShmSize != "" {
			shmSize, err := units.RAMInBytes(ipc.ShmSize)
			if err != nil {
				return nil, fmt.Errorf("failed to parse ShmSize: %v", err)
			}
			c.HostConfig.ShmSize = shmSize
		}
	}

	cs := new(ContainerState)
	cs.Restarting = n.Labels[restart.StatusLabel] == string(containerd.Running)
	cs.Error = n.Labels[labels.Error]
	if n.Process != nil {
		cs.Status = statusFromNative(n.Process.Status, n.Labels)
		cs.Running = n.Process.Status.Status == containerd.Running
		cs.Paused = n.Process.Status.Status == containerd.Paused
		cs.Pid = n.Process.Pid
		cs.ExitCode = int(n.Process.Status.ExitStatus)
		if containerAnnotations[labels.StateDir] != "" {
			if lf, err := state.New(containerAnnotations[labels.StateDir]); err != nil {
				log.L.WithError(err).Errorf("failed retrieving state")
			} else if err = lf.Load(); err != nil {
				log.L.WithError(err).Errorf("failed retrieving StartedAt from state")
			} else if !time.Time.IsZero(lf.StartedAt) {
				cs.StartedAt = lf.StartedAt.UTC().Format(time.RFC3339Nano)
			}
		}
		if !n.Process.Status.ExitTime.IsZero() {
			cs.FinishedAt = n.Process.Status.ExitTime.Format(time.RFC3339Nano)
		}
		nSettings, err := networkSettingsFromNative(n.Process.NetNS, n.Spec.(*specs.Spec))
		if err != nil {
			return nil, err
		}
		c.NetworkSettings = nSettings
		c.HostConfig.PortBindings = *nSettings.Ports
	} else {
		// n.process is not set if the container is not started, making the networkSetting null
		// we should send an empty object even in this case inorder for it to be compatible with docker inspect response
		nSettings, err := networkSettingsFromNative(nil, n.Spec.(*specs.Spec))
		if err != nil {
			return nil, err
		}
		c.NetworkSettings = nSettings
	}

	cpuSetting, err := cpuSettingsFromNative(n.Spec.(*specs.Spec))
	if err != nil {
		return nil, fmt.Errorf("failed to Decode cpuSetting: %v", err)
	}
	c.HostConfig.CPUSetCPUs = cpuSetting.CPUSetCpus
	c.HostConfig.CPUSetMems = cpuSetting.CPUSetMems
	c.HostConfig.CPUQuota = cpuSetting.CPUQuota
	c.HostConfig.CPUShares = cpuSetting.CPUShares
	c.HostConfig.CPUPeriod = cpuSetting.CPUPeriod
	c.HostConfig.CPURealtimePeriod = cpuSetting.CPURealtimePeriod
	c.HostConfig.CPURealtimeRuntime = cpuSetting.CPURealtimeRuntime

	cgroupNamespace, err := getCgroupnsFromNative(n.Spec.(*specs.Spec))
	if err != nil {
		return nil, fmt.Errorf("failed to Decode cgroupNamespace: %v", err)
	}
	c.HostConfig.CgroupnsMode = cgroupNamespace

	memorySettings, err := getMemorySettingsFromNative(n.Spec.(*specs.Spec))
	if err != nil {
		return nil, fmt.Errorf("failed to Decode memory Settings: %v", err)
	}

	c.HostConfig.OomKillDisable = memorySettings.DisableOOMKiller
	c.HostConfig.Memory = memorySettings.Limit
	c.HostConfig.MemorySwap = memorySettings.Swap

	dnsSettings, err := getDNSFromNative(n.Labels)
	if err != nil {
		return nil, fmt.Errorf("failed to Decode dns Settings: %v", err)
	}

	c.HostConfig.DNS = dnsSettings.DNSServers
	c.HostConfig.DNSOptions = dnsSettings.DNSResolvConfOptions
	c.HostConfig.DNSSearch = dnsSettings.DNSSearchDomains

	oomScoreAdj, err := getOomScoreAdjFromNative(n.Spec.(*specs.Spec))
	if err != nil {
		return nil, fmt.Errorf("failed to get OomScoreAdj value: %v", err)
	}
	c.HostConfig.OomScoreAdj = oomScoreAdj

	c.HostConfig.ReadonlyRootfs = false
	if n.Spec.(*specs.Spec).Root != nil && n.Spec.(*specs.Spec).Root.Readonly {
		c.HostConfig.ReadonlyRootfs = n.Spec.(*specs.Spec).Root.Readonly
	}

	utsMode, err := getUtsModeFromNative(n.Spec.(*specs.Spec))
	if err != nil {
		return nil, fmt.Errorf("failed to get UtsMode value: %v", err)
	}
	c.HostConfig.UTSMode = utsMode

	sysctls, err := getSysctlFromNative(n.Spec.(*specs.Spec))
	if err != nil {
		return nil, fmt.Errorf("failed to get UtsMode value: %v", err)
	}
	c.HostConfig.Sysctls = sysctls

	if n.Runtime.Name != "" {
		c.HostConfig.Runtime = n.Runtime.Name
	}

	c.State = cs
	c.Config = &Config{
		Labels: n.Labels,
	}
	if n.Labels[labels.Hostname] != "" {
		hostname = n.Labels[labels.Hostname]
	}
	c.Config.Hostname = hostname

	if n.Labels[labels.Domainname] != "" {
		c.Config.Domainname = n.Labels[labels.Domainname]
	}

	c.HostConfig.Devices = hostConfigLabel.Devices

	var pidMode string
	if n.Labels[labels.PIDContainer] != "" {
		pidMode = n.Labels[labels.PIDContainer]
	}
	c.HostConfig.PidMode = pidMode

	if err := getBlkioSettingsFromSpec(n.Spec.(*specs.Spec), c.HostConfig); err != nil {
		return nil, fmt.Errorf("failed to get blkio settings: %w", err)
	}

	if n.Spec != nil {
		if spec, ok := n.Spec.(*specs.Spec); ok && spec.Process != nil {
			c.Config.Env = spec.Process.Env
		}
	}

	if n.Labels[labels.User] != "" {
		c.Config.User = n.Labels[labels.User]
	}

	return c, nil
}

func ImageFromNative(nativeImage *native.Image) (*Image, error) {
	imgOCI := nativeImage.ImageConfig
	repository, tag := imgutil.ParseRepoTag(nativeImage.Image.Name)

	image := &Image{
		// Docker ID (digest of platform-specific config), not containerd ID (digest of multi-platform index or manifest)
		ID:           nativeImage.ImageConfigDesc.Digest.String(),
		Parent:       nativeImage.Image.Labels["org.mobyproject.image.parent"],
		Architecture: imgOCI.Architecture,
		Variant:      imgOCI.Platform.Variant,
		Os:           imgOCI.OS,
		Size:         nativeImage.Size,
		VirtualSize:  nativeImage.Size,
		RepoTags:     []string{fmt.Sprintf("%s:%s", repository, tag)},
		RepoDigests:  []string{fmt.Sprintf("%s@%s", repository, nativeImage.Image.Target.Digest.String())},
	}

	if len(imgOCI.History) > 0 {
		image.Comment = imgOCI.History[len(imgOCI.History)-1].Comment
		if !imgOCI.History[len(imgOCI.History)-1].Created.IsZero() {
			image.Created = imgOCI.History[len(imgOCI.History)-1].Created.Format(time.RFC3339Nano)
		}
		image.Author = imgOCI.History[len(imgOCI.History)-1].Author
	}

	image.RootFS.Type = imgOCI.RootFS.Type
	for _, d := range imgOCI.RootFS.DiffIDs {
		image.RootFS.Layers = append(image.RootFS.Layers, d.String())
	}

	portSet := make(nat.PortSet)
	for k := range imgOCI.Config.ExposedPorts {
		portSet[nat.Port(k)] = struct{}{}
	}

	image.Config = &Config{
		Cmd:          imgOCI.Config.Cmd,
		Volumes:      imgOCI.Config.Volumes,
		Env:          imgOCI.Config.Env,
		User:         imgOCI.Config.User,
		WorkingDir:   imgOCI.Config.WorkingDir,
		Entrypoint:   imgOCI.Config.Entrypoint,
		Labels:       imgOCI.Config.Labels,
		ExposedPorts: portSet,
	}

	return image, nil
}

// mountsFromNative only filters bind mount to transform from native container.
// Because native container shows all types of mounts, such as tmpfs, proc, sysfs.
func mountsFromNative(spMounts []specs.Mount) []MountPoint {
	mountpoints := make([]MountPoint, 0, len(spMounts))
	for _, m := range spMounts {
		var mp MountPoint
		if m.Type != "bind" {
			continue
		}
		mp.Type = m.Type
		mp.Source = m.Source
		mp.Destination = m.Destination
		mp.Mode = strings.Join(m.Options, ",")
		mp.RW, mp.Propagation = ParseMountProperties(m.Options)
		mountpoints = append(mountpoints, mp)
	}

	return mountpoints
}

func statusFromNative(x containerd.Status, labels map[string]string) string {
	switch s := x.Status; s {
	case containerd.Stopped:
		if labels[restart.StatusLabel] == string(containerd.Running) && restart.Reconcile(x, labels) {
			return "restarting"
		}
		return "exited"
	default:
		return string(s)
	}
}

func networkSettingsFromNative(n *native.NetNS, sp *specs.Spec) (*NetworkSettings, error) {
	res := &NetworkSettings{
		Networks: make(map[string]*NetworkEndpointSettings),
	}
	resPortMap := make(nat.PortMap)
	res.Ports = &resPortMap
	if n == nil {
		return res, nil
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
				log.L.WithError(err).WithField("name", x.Name).Warnf("failed to parse %q", a)
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
			var ports []cni.PortMapping
			err := json.Unmarshal([]byte(portsLabel), &ports)
			if err != nil {
				return nil, err
			}
			nports, err := convertToNatPort(ports)
			if err != nil {
				return nil, err
			}
			for portLabel, portBindings := range *nports {
				resPortMap[portLabel] = portBindings
			}
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

func cpuSettingsFromNative(sp *specs.Spec) (*CPUSettings, error) {
	res := &CPUSettings{}
	if sp.Linux != nil && sp.Linux.Resources != nil && sp.Linux.Resources.CPU != nil {
		if sp.Linux.Resources.CPU.Cpus != "" {
			res.CPUSetCpus = sp.Linux.Resources.CPU.Cpus
		}

		if sp.Linux.Resources.CPU.Mems != "" {
			res.CPUSetMems = sp.Linux.Resources.CPU.Mems
		}

		if sp.Linux.Resources.CPU.Shares != nil && *sp.Linux.Resources.CPU.Shares > 0 {
			res.CPUShares = *sp.Linux.Resources.CPU.Shares
		}

		if sp.Linux.Resources.CPU.Quota != nil && *sp.Linux.Resources.CPU.Quota > 0 {
			res.CPUQuota = *sp.Linux.Resources.CPU.Quota
		}
		if sp.Linux.Resources.CPU.Period != nil && *sp.Linux.Resources.CPU.Period > 0 {
			res.CPUPeriod = *sp.Linux.Resources.CPU.Period
		}
		if sp.Linux.Resources.CPU.RealtimePeriod != nil && *sp.Linux.Resources.CPU.RealtimePeriod > 0 {
			res.CPURealtimePeriod = *sp.Linux.Resources.CPU.RealtimePeriod
		}
		if sp.Linux.Resources.CPU.RealtimeRuntime != nil && *sp.Linux.Resources.CPU.RealtimeRuntime > 0 {
			res.CPURealtimeRuntime = *sp.Linux.Resources.CPU.RealtimeRuntime
		}
	}

	return res, nil
}

func getCgroupnsFromNative(sp *specs.Spec) (string, error) {
	res := ""
	if sp.Linux != nil && len(sp.Linux.Namespaces) != 0 {
		for _, ns := range sp.Linux.Namespaces {
			if ns.Type == "cgroup" {
				res = "private"
			}
		}
	}
	return res, nil
}

func groupAddFromNative(sp *specs.Spec) ([]string, error) {
	res := []string{}
	if sp.Process != nil && sp.Process.User.AdditionalGids != nil {
		for _, gid := range sp.Process.User.AdditionalGids {
			if gid != 0 {
				res = append(res, strconv.FormatUint(uint64(gid), 10))
			}
		}
	}
	return res, nil
}

func convertToNatPort(portMappings []cni.PortMapping) (*nat.PortMap, error) {
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

func parseExtraHosts(extraHostsJSON string) []string {
	var extraHosts []string
	if err := json.Unmarshal([]byte(extraHostsJSON), &extraHosts); err != nil {
		// Handle error or return empty slice
		return []string{}
	}
	return extraHosts
}

func getMemorySettingsFromNative(sp *specs.Spec) (*MemorySetting, error) {
	res := &MemorySetting{}
	if sp.Linux != nil && sp.Linux.Resources != nil && sp.Linux.Resources.Memory != nil {
		if sp.Linux.Resources.Memory.DisableOOMKiller != nil {
			res.DisableOOMKiller = *sp.Linux.Resources.Memory.DisableOOMKiller
		}

		if sp.Linux.Resources.Memory.Limit != nil {
			res.Limit = *sp.Linux.Resources.Memory.Limit
		}

		if sp.Linux.Resources.Memory.Swap != nil {
			res.Swap = *sp.Linux.Resources.Memory.Swap
		}
	}
	return res, nil
}

func getDNSFromNative(lbls map[string]string) (*DNSSettings, error) {
	res := &DNSSettings{}

	if dnsSettingJSON, ok := lbls[labels.DNSSetting]; ok {
		if err := json.Unmarshal([]byte(dnsSettingJSON), &res); err != nil {
			return nil, fmt.Errorf("failed to parse DNS settings: %v", err)
		}
	}

	return res, nil
}

func getHostConfigLabelFromNative(lbls map[string]string) (*HostConfigLabel, error) {
	res := &HostConfigLabel{}

	if hostConfigLabelJSON, ok := lbls[labels.HostConfigLabel]; ok {
		if err := json.Unmarshal([]byte(hostConfigLabelJSON), &res); err != nil {
			return nil, fmt.Errorf("failed to parse DNS servers: %v", err)
		}
	}
	return res, nil
}

func getOomScoreAdjFromNative(sp *specs.Spec) (int, error) {
	var res int
	if sp.Process != nil && sp.Process.OOMScoreAdj != nil {
		res = *sp.Process.OOMScoreAdj
	}
	return res, nil
}

func getUtsModeFromNative(sp *specs.Spec) (string, error) {
	if sp.Linux != nil && len(sp.Linux.Namespaces) > 0 {
		for _, ns := range sp.Linux.Namespaces {
			if ns.Type == "uts" {
				return "", nil
			}
		}
	}
	return "host", nil
}

func getSysctlFromNative(sp *specs.Spec) (map[string]string, error) {
	var res map[string]string
	if sp.Linux != nil && sp.Linux.Sysctl != nil {
		res = sp.Linux.Sysctl
	}
	return res, nil
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
	Name       string                      `json:"Name"`
	ID         string                      `json:"Id,omitempty"` // optional in nerdctl
	IPAM       IPAM                        `json:"IPAM,omitempty"`
	Labels     map[string]string           `json:"Labels"`
	Containers map[string]EndpointResource `json:"Containers"` // Containers contains endpoints belonging to the network
	// Scope, Driver, etc. are omitted
}

type EndpointResource struct {
	Name string `json:"Name"`
	// EndpointID  string `json:"EndpointID"`
	// MacAddress  string `json:"MacAddress"`
	// IPv4Address string `json:"IPv4Address"`
	// IPv6Address string `json:"IPv6Address"`
}

type structuredCNI struct {
	Name    string `json:"name"`
	Plugins []struct {
		Ipam struct {
			Ranges [][]IPAMConfig `json:"ranges"`
		} `json:"ipam"`
	} `json:"plugins"`
}

type MemorySetting struct {
	Limit            int64 `json:"limit"`
	Swap             int64 `json:"swap"`
	DisableOOMKiller bool  `json:"disableOOMKiller"`
}

func NetworkFromNative(n *native.Network) (*Network, error) {
	var res Network

	sCNI := &structuredCNI{}
	err := json.Unmarshal(n.CNI, sCNI)
	if err != nil {
		return nil, err
	}

	res.Name = sCNI.Name
	for _, plugin := range sCNI.Plugins {
		for _, ranges := range plugin.Ipam.Ranges {
			res.IPAM.Config = append(res.IPAM.Config, ranges...)
		}
	}

	if n.NerdctlID != nil {
		res.ID = *n.NerdctlID
	}

	if n.NerdctlLabels != nil {
		res.Labels = *n.NerdctlLabels
	}

	res.Containers = make(map[string]EndpointResource)
	for _, container := range n.Containers {
		res.Containers[container.ID] = EndpointResource{
			Name: container.Labels[labels.Name],
			// EndpointID:  container.EndpointID,
			// MacAddress:  container.MacAddress,
			// IPv4Address: container.IPv4Address,
			// IPv6Address: container.IPv6Address,
		}
	}

	return &res, nil
}

func parseMounts(nerdctlMounts string) ([]MountPoint, error) {
	var mounts []MountPoint
	err := json.Unmarshal([]byte(nerdctlMounts), &mounts)
	if err != nil {
		return nil, err
	}

	return mounts, nil
}

func ParseMountProperties(option []string) (rw bool, propagation string) {
	rw = true
	for _, opt := range option {
		switch opt {
		case "ro", "rro":
			rw = false
		case "private", "rprivate", "shared", "rshared", "slave", "rslave":
			propagation = opt
		default:
		}
	}
	return
}

func getDefaultLinuxBlkioSettings() LinuxBlkioSettings {
	return LinuxBlkioSettings{
		BlkioWeight:          0,
		BlkioWeightDevice:    make([]*specs.LinuxWeightDevice, 0),
		BlkioDeviceReadBps:   make([]*specs.LinuxThrottleDevice, 0),
		BlkioDeviceWriteBps:  make([]*specs.LinuxThrottleDevice, 0),
		BlkioDeviceReadIOps:  make([]*specs.LinuxThrottleDevice, 0),
		BlkioDeviceWriteIOps: make([]*specs.LinuxThrottleDevice, 0),
	}
}

func getBlkioSettingsFromSpec(spec *specs.Spec, hostConfig *HostConfig) error {
	if spec == nil {
		return fmt.Errorf("spec cannot be nil")
	}
	if hostConfig == nil {
		return fmt.Errorf("hostConfig cannot be nil")
	}

	// Initialize empty arrays by default
	hostConfig.LinuxBlkioSettings = getDefaultLinuxBlkioSettings()

	if spec.Linux == nil || spec.Linux.Resources == nil || spec.Linux.Resources.BlockIO == nil {
		return nil
	}

	blockIO := spec.Linux.Resources.BlockIO

	// Set block IO weight
	if blockIO.Weight != nil {
		hostConfig.BlkioWeight = *blockIO.Weight
	}

	// Set weight devices
	if len(blockIO.WeightDevice) > 0 {
		hostConfig.BlkioWeightDevice = make([]*specs.LinuxWeightDevice, len(blockIO.WeightDevice))
		for i, dev := range blockIO.WeightDevice {
			hostConfig.BlkioWeightDevice[i] = &dev
		}
	}

	// Set throttle devices for read BPS
	if len(blockIO.ThrottleReadBpsDevice) > 0 {
		hostConfig.BlkioDeviceReadBps = make([]*specs.LinuxThrottleDevice, len(blockIO.ThrottleReadBpsDevice))
		for i, dev := range blockIO.ThrottleReadBpsDevice {
			hostConfig.BlkioDeviceReadBps[i] = &dev
		}
	}

	// Set throttle devices for write BPS
	if len(blockIO.ThrottleWriteBpsDevice) > 0 {
		hostConfig.BlkioDeviceWriteBps = make([]*specs.LinuxThrottleDevice, len(blockIO.ThrottleWriteBpsDevice))
		for i, dev := range blockIO.ThrottleWriteBpsDevice {
			hostConfig.BlkioDeviceWriteBps[i] = &dev
		}
	}

	// Set throttle devices for read IOPs
	if len(blockIO.ThrottleReadIOPSDevice) > 0 {
		hostConfig.BlkioDeviceReadIOps = make([]*specs.LinuxThrottleDevice, len(blockIO.ThrottleReadIOPSDevice))
		for i, dev := range blockIO.ThrottleReadIOPSDevice {
			hostConfig.BlkioDeviceReadIOps[i] = &dev
		}
	}

	// Set throttle devices for write IOPs
	if len(blockIO.ThrottleWriteIOPSDevice) > 0 {
		hostConfig.BlkioDeviceWriteIOps = make([]*specs.LinuxThrottleDevice, len(blockIO.ThrottleWriteIOPSDevice))
		for i, dev := range blockIO.ThrottleWriteIOPSDevice {
			hostConfig.BlkioDeviceWriteIOps[i] = &dev
		}
	}
	return nil
}
