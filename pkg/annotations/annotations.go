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

// Package annotations provides a high-level interface to retrieve and manipulate containers OCI annotations.
// Any parsing, encoding or transformation of the struct into concrete containers annotations must be handled here,
// and the containers underlying annotations map should never be accessed directly by consuming code, to ensure
// we can evolve the storage format in a single place and consuming code to be fully isolated from the details of
// how it is being handled.
// Container annotations generally should be used to store state information or other container properties that
// we may want to inspect at different stages of the container lifecycle.
package annotations

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/opencontainers/runtime-spec/specs-go"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/go-cni"

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/mountutil"
	"github.com/containerd/nerdctl/v2/pkg/platformutil"
)

func New(ctx context.Context, con containerd.Container) (*Annotations, error) {
	an := &Annotations{}
	err := an.Unmarshall(ctx, con)
	return an, err
}

func NewFromState(state *specs.State) (*Annotations, error) {
	an := &Annotations{}
	if state == nil || state.Annotations == nil {
		return nil, fmt.Errorf("invalid state")
	}
	err := an.UnmarshallFromMap(state.Annotations)
	return an, err
}

type Log struct {
	Driver  string
	Opts    map[string]string
	Address string
}

type Annotations struct {
	AnonVolumes               []string
	Bypass4netns              bool
	Bypass4netnsIgnoreBind    bool
	Bypass4netnsIgnoreSubnets []string
	CidFile                   string
	DeviceMapping             []dockercompat.DeviceMapping
	DNSResolvConfOptions      []string
	DNSSearchDomains          []string
	DNSServers                []string
	DomainName                string
	ExtraHosts                map[string]string
	// GroupAdd             []string
	HostName         string
	IPC              string
	LogConfig        *Log
	LogURI           string
	MountPoints      []*mountutil.Processed
	Name             string
	Namespace        string
	NetworkNamespace string
	Networks         []string
	Platform         string
	StateDir         string
	PidContainer     string
	PidFile          string
	Ports            []cni.PortMapping
	Rm               bool
	User             string

	// FIXME: these should be replaced by a richer Network label allowing per network ip and mac
	IP6Address string
	IPAddress  string
	MACAddress string
}

func (an *Annotations) UnmarshallFromMap(source map[string]string) error {
	hostConfig := &dockercompat.HostConfig{}
	dnsConfig := &dockercompat.DNSSettings{}
	for k, v := range source {
		switch k {
		case AnonymousVolumes:
			_ = json.Unmarshal([]byte(v), &an.AnonVolumes)
		case Bypass4netns:
			an.Bypass4netns, _ = strconv.ParseBool(v)
		case Bypass4netnsIgnoreBind:
			an.Bypass4netnsIgnoreBind, _ = strconv.ParseBool(v)
		case Bypass4netnsIgnoreSubnets:
			_ = json.Unmarshal([]byte(v), &an.Bypass4netnsIgnoreSubnets)
		case DNSSetting:
			_ = json.Unmarshal([]byte(v), dnsConfig)
			an.DNSServers = dnsConfig.DNSServers
			an.DNSSearchDomains = dnsConfig.DNSSearchDomains
			an.DNSResolvConfOptions = dnsConfig.DNSResolvConfOptions
		case Domainname:
			an.DomainName = v
		case ExtraHosts:
			hosts := []string{}
			_ = json.Unmarshal([]byte(v), &hosts)
			if an.ExtraHosts == nil {
				an.ExtraHosts = map[string]string{}
			}
			for _, host := range hosts {
				if v := strings.SplitN(host, ":", 2); len(v) == 2 {
					an.ExtraHosts[v[0]] = v[1]
				}
			}

		case HostConfig:
			_ = json.Unmarshal([]byte(v), hostConfig)
			an.CidFile = hostConfig.ContainerIDFile
			//  = hostConfig.CgroupnsMode
		case Hostname:
			an.HostName = v
		case IP6Address:
			an.IP6Address = v
		case IPAddress:
			an.IPAddress = v
		case IPC:
			an.IPC = v
		case LogConfig:
			_ = json.Unmarshal([]byte(v), &an.LogConfig)
		case LogURI:
			an.LogURI = v
		case MACAddress:
			an.MACAddress = v
		case Mounts:
			_ = json.Unmarshal([]byte(v), &an.MountPoints)
		case Name:
			an.Name = v
		case Namespace:
			an.Namespace = v
		case NetworkNamespace:
			an.NetworkNamespace = v
		case Networks:
			_ = json.Unmarshal([]byte(v), &an.Networks)
		case PIDContainer:
			an.PidContainer = v
		case PIDFile:
			an.PidFile = v
		case Platform:
			an.Platform = v
		case Ports:
			_ = json.Unmarshal([]byte(v), &an.Ports)
		case ContainerAutoRemove:
			an.Rm, _ = strconv.ParseBool(v)
		case StateDir:
			an.StateDir = v
			// FIXME: add missing, including NetworkNamespace
		case User:
			an.User = v
		default:
		}
	}

	return nil
}

func (an *Annotations) Unmarshall(ctx context.Context, con containerd.Container) error {
	conSpecs, err := con.Spec(ctx)
	if err != nil || conSpecs.Annotations == nil {
		return err
	}

	return an.UnmarshallFromMap(conSpecs.Annotations)
}

func (an *Annotations) Marshall() (map[string]string, error) {
	annot := make(map[string]string)

	var (
		err         error
		hostConfig  dockercompat.HostConfigLabel
		dnsSettings dockercompat.DNSSettings
	)

	if len(an.AnonVolumes) > 0 {
		anonVolumeJSON, err := json.Marshal(an.AnonVolumes)
		if err != nil {
			return nil, err
		}
		annot[AnonymousVolumes] = string(anonVolumeJSON)
	}

	if an.CidFile != "" {
		hostConfig.CidFile = an.CidFile
	}

	if len(an.DeviceMapping) > 0 {
		hostConfig.Devices = append(hostConfig.Devices, an.DeviceMapping...)
	}

	if len(an.DNSResolvConfOptions) > 0 {
		dnsSettings.DNSResolvConfOptions = an.DNSResolvConfOptions
	}

	if len(an.DNSServers) > 0 {
		dnsSettings.DNSServers = an.DNSServers
	}

	if len(an.DNSSearchDomains) > 0 {
		dnsSettings.DNSSearchDomains = an.DNSSearchDomains
	}

	dnsSettingsJSON, err := json.Marshal(dnsSettings)
	if err != nil {
		return nil, err
	}
	annot[DNSSetting] = string(dnsSettingsJSON)

	annot[Domainname] = an.DomainName

	//if len(an.GroupAdd) > 0 {
	//}

	hosts := []string{}
	for k, v := range an.ExtraHosts {
		hosts = append(hosts, fmt.Sprintf("%s:%s", k, v))
	}
	extraHostsJSON, err := json.Marshal(hosts)
	if err != nil {
		return nil, err
	}
	annot[ExtraHosts] = string(extraHostsJSON)

	hostConfigJSON, err := json.Marshal(hostConfig)
	if err != nil {
		return nil, err
	}
	annot[HostConfig] = string(hostConfigJSON)

	annot[Hostname] = an.HostName

	if an.IP6Address != "" {
		annot[IP6Address] = an.IP6Address
	}

	if an.IPAddress != "" {
		annot[IPAddress] = an.IPAddress
	}

	if an.IPC != "" {
		annot[IPC] = an.IPC
	}

	if an.LogURI != "" {
		annot[LogURI] = an.LogURI

		if an.LogConfig != nil {
			logConfigJSON, err := json.Marshal(an.LogConfig)
			if err != nil {
				return nil, err
			}

			annot[LogConfig] = string(logConfigJSON)
		}
	}

	if an.MACAddress != "" {
		annot[MACAddress] = an.MACAddress
	}

	if len(an.MountPoints) > 0 {
		mounts := dockercompatMounts(an.MountPoints)
		mountPointsJSON, err := json.Marshal(mounts)
		if err != nil {
			return nil, err
		}
		annot[Mounts] = string(mountPointsJSON)
	}

	annot[Name] = an.Name
	annot[Namespace] = an.Namespace

	networksJSON, err := json.Marshal(an.Networks)
	if err != nil {
		return nil, err
	}
	annot[Networks] = string(networksJSON)

	annot[Platform], err = platformutil.NormalizeString(an.Platform)
	if err != nil {
		return nil, err
	}

	if an.PidFile != "" {
		annot[PIDFile] = an.PidFile
	}

	if len(an.Ports) > 0 {
		portsJSON, err := json.Marshal(an.Ports)
		if err != nil {
			return nil, err
		}

		annot[Ports] = string(portsJSON)
	}

	if an.PidContainer != "" {
		annot[PIDContainer] = an.PidContainer
	}

	annot[ContainerAutoRemove] = fmt.Sprintf("%t", an.Rm)

	annot[StateDir] = an.StateDir

	if an.User != "" {
		annot[User] = an.User
	}

	return annot, nil
}

func dockercompatMounts(mountPoints []*mountutil.Processed) []dockercompat.MountPoint {
	result := make([]dockercompat.MountPoint, len(mountPoints))
	for i := range mountPoints {
		mp := mountPoints[i]
		result[i] = dockercompat.MountPoint{
			Type:        mp.Type,
			Name:        mp.Name,
			Source:      mp.Mount.Source,
			Destination: mp.Mount.Destination,
			Driver:      "",
			Mode:        mp.Mode,
		}
		result[i].RW, result[i].Propagation = dockercompat.ParseMountProperties(strings.Split(mp.Mode, ","))

		// it's an anonymous volume
		if mp.AnonymousVolume != "" {
			result[i].Name = mp.AnonymousVolume
		}

		// volume only support local driver
		if mp.Type == "volume" {
			result[i].Driver = "local"
		}
	}

	return result
}
