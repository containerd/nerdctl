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

package infoutil

import (
	"context"
	"os"
	"runtime"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/pkg/apparmor"
	"github.com/containerd/containerd/services/introspection"
	"github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/containerd/nerdctl/pkg/version"
	ptypes "github.com/gogo/protobuf/types"
)

func Info(ctx context.Context, client *containerd.Client, snapshotter, cgroupManager string) (*dockercompat.Info, error) {
	daemonVersion, err := client.Version(ctx)
	if err != nil {
		return nil, err
	}
	introService := client.IntrospectionService()
	daemonIntro, err := introService.Server(ctx, &ptypes.Empty{})
	if err != nil {
		return nil, err
	}
	snapshotterPlugins, err := GetSnapshotterNames(ctx, introService)
	if err != nil {
		return nil, err
	}

	var info dockercompat.Info
	info.ID = daemonIntro.UUID
	// Storage Driver is not really Server concept for nerdctl, but mimics `docker info` output
	info.Driver = snapshotter
	info.Plugins.Log = []string{"json-file"}
	info.Plugins.Storage = snapshotterPlugins
	info.LoggingDriver = "json-file" // hard-coded
	info.CgroupDriver = cgroupManager
	info.CgroupVersion = CgroupsVersion()
	info.KernelVersion = UnameR()
	info.OperatingSystem = DistroName()
	info.OSType = runtime.GOOS
	info.Architecture = UnameM()
	info.Name, err = os.Hostname()
	if err != nil {
		return nil, err
	}
	info.ServerVersion = daemonVersion.Version
	if apparmor.HostSupports() {
		info.SecurityOptions = append(info.SecurityOptions, "name=apparmor")
	}
	info.SecurityOptions = append(info.SecurityOptions, "name=seccomp,profile=default")
	if defaults.CgroupnsMode() == "private" {
		info.SecurityOptions = append(info.SecurityOptions, "name=cgroupns")
	}
	if rootlessutil.IsRootlessChild() {
		info.SecurityOptions = append(info.SecurityOptions, "name=rootless")
	}
	return &info, nil
}

func GetSnapshotterNames(ctx context.Context, introService introspection.Service) ([]string, error) {
	var names []string
	plugins, err := introService.Plugins(ctx, nil)
	if err != nil {
		return nil, err
	}
	for _, p := range plugins.Plugins {
		if strings.HasPrefix(p.Type, "io.containerd.snapshotter.") && p.InitErr == nil {
			names = append(names, p.ID)
		}
	}
	return names, nil
}

func ClientVersion() dockercompat.ClientVersion {
	return dockercompat.ClientVersion{
		Version:   version.Version,
		GitCommit: version.Revision,
		GoVersion: runtime.Version(),
		Os:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

func ServerVersion(ctx context.Context, client *containerd.Client) (*dockercompat.ServerVersion, error) {
	daemonVersion, err := client.Version(ctx)
	if err != nil {
		return nil, err
	}

	v := &dockercompat.ServerVersion{
		Components: []dockercompat.ComponentVersion{
			{
				Name:    "containerd",
				Version: daemonVersion.Version,
				Details: map[string]string{"GitCommit": daemonVersion.Revision},
			},
		},
		// TODO: add runc version
	}
	return v, nil
}
