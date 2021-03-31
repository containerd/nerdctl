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
	"bufio"
	"context"
	"io"
	"os"
	"regexp"
	"runtime"
	"strings"

	"github.com/containerd/cgroups"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/pkg/apparmor"
	"github.com/containerd/containerd/services/introspection"
	"github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	ptypes "github.com/gogo/protobuf/types"
	"golang.org/x/sys/unix"
)

// UnameR returns `uname -r`
func UnameR() string {
	var utsname unix.Utsname
	if err := unix.Uname(&utsname); err != nil {
		// error is unlikely to happen
		return ""
	}
	var s string
	for _, f := range utsname.Release {
		if f == 0 {
			break
		}
		s += string(f)
	}
	return s
}

// UnameM returns `uname -m`
func UnameM() string {
	var utsname unix.Utsname
	if err := unix.Uname(&utsname); err != nil {
		// error is unlikely to happen
		return ""
	}
	var s string
	for _, f := range utsname.Machine {
		if f == 0 {
			break
		}
		s += string(f)
	}
	return s
}

const UnameO = "GNU/Linux"

func DistroName() string {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return UnameO
	}
	defer f.Close()
	return distroName(f)
}

func distroName(r io.Reader) string {
	scanner := bufio.NewScanner(r)
	var name, version string
	for scanner.Scan() {
		line := scanner.Text()
		k, v := getOSReleaseAttrib(line)
		switch k {
		case "PRETTY_NAME":
			return v
		case "NAME":
			name = v
		case "VERSION":
			version = v
		}
	}
	if name != "" {
		if version != "" {
			return name + " " + version
		}
		return name
	}
	return UnameO
}

var osReleaseAttribRegex = regexp.MustCompile(`([^\s=]+)\s*=\s*("{0,1})([^"]*)("{0,1})`)

func getOSReleaseAttrib(line string) (string, string) {
	splitBySlash := strings.SplitN(line, "#", 2)
	l := strings.TrimSpace(splitBySlash[0])
	x := osReleaseAttribRegex.FindAllStringSubmatch(l, -1)
	if len(x) >= 1 && len(x[0]) > 3 {
		return x[0][1], x[0][3]
	}
	return "", ""
}

func Info(ctx context.Context, client *containerd.Client, defaultSnapshotter string) (*dockercompat.Info, error) {
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
	info.Driver = defaultSnapshotter
	info.Plugins.Log = []string{"json-file"}
	info.Plugins.Storage = snapshotterPlugins
	info.LoggingDriver = "json-file" // hard-coded
	info.CgroupDriver = defaults.CgroupManager()
	info.CgroupVersion = "1"
	if cgroups.Mode() == cgroups.Unified {
		info.CgroupVersion = "2"
	}
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
