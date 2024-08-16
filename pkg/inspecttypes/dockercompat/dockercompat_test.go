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

package dockercompat

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/docker/go-connections/nat"
	"github.com/opencontainers/runtime-spec/specs-go"
	"gotest.tools/v3/assert"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/containers"

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/native"
)

func TestContainerFromNative(t *testing.T) {
	tempStateDir, err := os.MkdirTemp(t.TempDir(), "rw")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(tempStateDir, "resolv.conf"), []byte(""), 0644)
	defer os.RemoveAll(tempStateDir)

	testcase := []struct {
		name     string
		n        *native.Container
		expected *Container
	}{
		// nerdctl container, mount /mnt/foo:/mnt/foo:rw,rslave; ResolvConfPath; hostname
		{
			name: "container from nerdctl",
			n: &native.Container{
				Container: containers.Container{
					Labels: map[string]string{
						"nerdctl/mounts":    "[{\"Type\":\"bind\",\"Source\":\"/mnt/foo\",\"Destination\":\"/mnt/foo\",\"Mode\":\"rshared,rw\",\"RW\":true,\"Propagation\":\"rshared\"}]",
						"nerdctl/state-dir": tempStateDir,
						"nerdctl/hostname":  "host1",
					},
				},
				Spec: &specs.Spec{},
				Process: &native.Process{
					Pid: 10000,
					Status: containerd.Status{
						Status: "running",
					},
				},
			},
			expected: &Container{
				Created:        "0001-01-01T00:00:00Z",
				Platform:       runtime.GOOS,
				ResolvConfPath: tempStateDir + "/resolv.conf",
				State: &ContainerState{
					Status:     "running",
					Running:    true,
					Pid:        10000,
					FinishedAt: "",
				},
				Mounts: []MountPoint{
					{
						Type:        "bind",
						Source:      "/mnt/foo",
						Destination: "/mnt/foo",
						Mode:        "rshared,rw",
						RW:          true,
						Propagation: "rshared",
					},
				},
				Config: &Config{
					Labels: map[string]string{
						"nerdctl/mounts":    "[{\"Type\":\"bind\",\"Source\":\"/mnt/foo\",\"Destination\":\"/mnt/foo\",\"Mode\":\"rshared,rw\",\"RW\":true,\"Propagation\":\"rshared\"}]",
						"nerdctl/state-dir": tempStateDir,
						"nerdctl/hostname":  "host1",
					},
					Hostname: "host1",
				},
				NetworkSettings: &NetworkSettings{
					Ports:    &nat.PortMap{},
					Networks: map[string]*NetworkEndpointSettings{},
				},
			},
		},
		// cri container, mount /mnt/foo:/mnt/foo:rw,rslave; mount resolv.conf and hostname; internal sysfs mount
		{
			name: "container from cri",
			n: &native.Container{
				Container: containers.Container{},
				Spec: &specs.Spec{
					Mounts: []specs.Mount{
						{
							Destination: "/etc/resolv.conf",
							Type:        "bind",
							Source:      "/mock-sandbox-dir/resolv.conf",
							Options:     []string{"rbind", "rprivate", "rw"},
						},
						{
							Destination: "/etc/hostname",
							Type:        "bind",
							Source:      "/mock-sandbox-dir/hostname",
							Options:     []string{"rbind", "rprivate", "rw"},
						},
						{
							Destination: "/mnt/foo",
							Type:        "bind",
							Source:      "/mnt/foo",
							Options:     []string{"rbind", "rslave", "rw"},
						},
						{
							Destination: "/sys",
							Type:        "sysfs",
							Source:      "sysfs",
							Options:     []string{"nosuid", "noexec", "nodev", "ro"},
						},
					},
				},
				Process: &native.Process{
					Pid: 10000,
					Status: containerd.Status{
						Status: "running",
					},
				},
			},
			expected: &Container{
				Created:        "0001-01-01T00:00:00Z",
				Platform:       runtime.GOOS,
				ResolvConfPath: "/mock-sandbox-dir/resolv.conf",
				HostnamePath:   "/mock-sandbox-dir/hostname",
				State: &ContainerState{
					Status:     "running",
					Running:    true,
					Pid:        10000,
					FinishedAt: "",
				},
				Mounts: []MountPoint{
					{
						Type:        "bind",
						Source:      "/mock-sandbox-dir/resolv.conf",
						Destination: "/etc/resolv.conf",
						Mode:        "rbind,rprivate,rw",
						RW:          true,
						Propagation: "rprivate",
					},
					{
						Type:        "bind",
						Source:      "/mock-sandbox-dir/hostname",
						Destination: "/etc/hostname",
						Mode:        "rbind,rprivate,rw",
						RW:          true,
						Propagation: "rprivate",
					},
					{
						Type:        "bind",
						Source:      "/mnt/foo",
						Destination: "/mnt/foo",
						Mode:        "rbind,rslave,rw",
						RW:          true,
						Propagation: "rslave",
					},
					// ignore sysfs mountpoint
				},
				Config: &Config{},
				NetworkSettings: &NetworkSettings{
					Ports:    &nat.PortMap{},
					Networks: map[string]*NetworkEndpointSettings{},
				},
			},
		},
		// ctr container, mount /mnt/foo:/mnt/foo:rw,rslave; internal sysfs mount; hostname
		{
			name: "container from ctr",
			n: &native.Container{
				Container: containers.Container{},
				Spec: &specs.Spec{
					Hostname: "host1",
					Mounts: []specs.Mount{
						{
							Destination: "/mnt/foo",
							Type:        "bind",
							Source:      "/mnt/foo",
							Options:     []string{"rbind", "rslave", "rw"},
						},
						{
							Destination: "/sys",
							Type:        "sysfs",
							Source:      "sysfs",
							Options:     []string{"nosuid", "noexec", "nodev", "ro"},
						},
					},
				},
				Process: &native.Process{
					Pid: 10000,
					Status: containerd.Status{
						Status: "running",
					},
				},
			},
			expected: &Container{
				Created:  "0001-01-01T00:00:00Z",
				Platform: runtime.GOOS,
				State: &ContainerState{
					Status:     "running",
					Running:    true,
					Pid:        10000,
					FinishedAt: "",
				},
				Mounts: []MountPoint{
					{
						Type:        "bind",
						Source:      "/mnt/foo",
						Destination: "/mnt/foo",
						Mode:        "rbind,rslave,rw",
						RW:          true,
						Propagation: "rslave",
					},
					// ignore sysfs mountpoint
				},
				Config: &Config{
					Hostname: "host1",
				},
				NetworkSettings: &NetworkSettings{
					Ports:    &nat.PortMap{},
					Networks: map[string]*NetworkEndpointSettings{},
				},
			},
		},
	}

	for _, tc := range testcase {
		t.Run(tc.name, func(tt *testing.T) {
			d, _ := ContainerFromNative(tc.n)
			assert.DeepEqual(tt, d, tc.expected)
		})
	}
}

func TestNetworkSettingsFromNative(t *testing.T) {
	tempStateDir, err := os.MkdirTemp(t.TempDir(), "rw")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(tempStateDir, "resolv.conf"), []byte(""), 0644)
	defer os.RemoveAll(tempStateDir)

	testcase := []struct {
		name     string
		n        *native.NetNS
		s        *specs.Spec
		expected *NetworkSettings
	}{
		// Given null native.NetNS, Return initialized NetworkSettings
		//    UseCase: Inspect a Stopped Container
		{
			name: "Given Null NetNS, Return initialized NetworkSettings",
			n:    nil,
			s:    &specs.Spec{},
			expected: &NetworkSettings{
				Ports:    &nat.PortMap{},
				Networks: map[string]*NetworkEndpointSettings{},
			},
		},
		// Given native.NetNS with single Interface with Port Annotations, Return populated NetworkSettings
		//   UseCase: Inspect a Running Container with published ports
		{
			name: "Given NetNS with single Interface with Port Annotation, Return populated NetworkSettings",
			n: &native.NetNS{
				Interfaces: []native.NetInterface{
					{
						Interface: net.Interface{
							Index: 1,
							MTU:   1500,
							Name:  "eth0.100",
							Flags: net.FlagUp,
						},
						HardwareAddr: "xx:xx:xx:xx:xx:xx",
						Flags:        []string{},
						Addrs:        []string{"10.0.4.30/24"},
					},
				},
			},
			s: &specs.Spec{
				Annotations: map[string]string{
					"nerdctl/ports": "[{\"HostPort\":8075,\"ContainerPort\":77,\"Protocol\":\"tcp\",\"HostIP\":\"127.0.0.1\"}]",
				},
			},
			expected: &NetworkSettings{
				Ports: &nat.PortMap{
					nat.Port("77/tcp"): []nat.PortBinding{
						{
							HostIP:   "127.0.0.1",
							HostPort: "8075",
						},
					},
				},
				Networks: map[string]*NetworkEndpointSettings{
					"unknown-eth0.100": {
						IPAddress:   "10.0.4.30",
						IPPrefixLen: 24,
						MacAddress:  "xx:xx:xx:xx:xx:xx",
					},
				},
			},
		},
		// Given native.NetNS with single Interface without Port Annotations, Return valid NetworkSettings w/ empty Ports
		//   UseCase: Inspect a Running Container without published ports
		{
			name: "Given NetNS with single Interface without Port Annotations, Return valid NetworkSettings w/ empty Ports",
			n: &native.NetNS{
				Interfaces: []native.NetInterface{
					{
						Interface: net.Interface{
							Index: 1,
							MTU:   1500,
							Name:  "eth0.100",
							Flags: net.FlagUp,
						},
						HardwareAddr: "xx:xx:xx:xx:xx:xx",
						Flags:        []string{},
						Addrs:        []string{"10.0.4.30/24"},
					},
				},
			},
			s: &specs.Spec{
				Annotations: map[string]string{},
			},
			expected: &NetworkSettings{
				Ports: &nat.PortMap{},
				Networks: map[string]*NetworkEndpointSettings{
					"unknown-eth0.100": {
						IPAddress:   "10.0.4.30",
						IPPrefixLen: 24,
						MacAddress:  "xx:xx:xx:xx:xx:xx",
					},
				},
			},
		},
	}

	for _, tc := range testcase {
		t.Run(tc.name, func(tt *testing.T) {
			d, _ := networkSettingsFromNative(tc.n, tc.s)
			assert.DeepEqual(tt, d, tc.expected)
		})
	}
}
