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
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/containerd/containerd"
	ptypes "github.com/containerd/containerd/protobuf/types"
	"github.com/containerd/containerd/services/introspection"
	"github.com/containerd/nerdctl/pkg/buildkitutil"
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/pkg/logging"
	"github.com/containerd/nerdctl/pkg/version"
	"github.com/sirupsen/logrus"
)

func NativeDaemonInfo(ctx context.Context, client *containerd.Client) (*native.DaemonInfo, error) {
	introService := client.IntrospectionService()
	plugins, err := introService.Plugins(ctx, nil)
	if err != nil {
		return nil, err
	}
	server, err := introService.Server(ctx, &ptypes.Empty{})
	if err != nil {
		return nil, err
	}
	versionService := client.VersionService()
	version, err := versionService.Version(ctx, &ptypes.Empty{})
	if err != nil {
		return nil, err
	}

	daemonInfo := &native.DaemonInfo{
		Plugins: plugins,
		Server:  server,
		Version: version,
	}
	return daemonInfo, nil
}

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
	// Storage drivers and logging drivers are not really Server concept for nerdctl, but mimics `docker info` output
	info.Driver = snapshotter
	info.Plugins.Log = logging.Drivers()
	info.Plugins.Storage = snapshotterPlugins
	info.SystemTime = time.Now().Format(time.RFC3339Nano)
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
	fulfillPlatformInfo(&info)
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
		Components: []dockercompat.ComponentVersion{
			buildctlVersion(),
		},
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
			runcVersion(),
		},
	}
	return v, nil
}

func ServerSemVer(ctx context.Context, client *containerd.Client) (*semver.Version, error) {
	v, err := client.Version(ctx)
	if err != nil {
		return nil, err
	}
	sv, err := semver.NewVersion(v.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the containerd version %q: %w", v.Version, err)
	}
	return sv, nil
}

func buildctlVersion() dockercompat.ComponentVersion {
	const buildctl = "buildctl"
	buildctlBinary, err := buildkitutil.BuildctlBinary()
	if err != nil {
		logrus.Warnf("unable to determine buildctl version: %s", err.Error())
		return dockercompat.ComponentVersion{Name: buildctl}
	}

	stdout, err := exec.Command(buildctlBinary, "--version").Output()
	if err != nil {
		logrus.Warnf("unable to determine buildctl version: %s", err.Error())
		return dockercompat.ComponentVersion{Name: buildctl}
	}

	versionStr := strings.Fields(strings.TrimSpace(string(stdout)))
	if len(versionStr) != 4 {
		logrus.Errorf("unable to determine buildctl version got: %s", versionStr)
		return dockercompat.ComponentVersion{Name: buildctl}
	}

	return dockercompat.ComponentVersion{
		Name:    buildctl,
		Version: versionStr[2],
		Details: map[string]string{"GitCommit": versionStr[3]},
	}
}

func runcVersion() dockercompat.ComponentVersion {
	stdout, err := exec.Command("runc", "--version").Output()
	if err != nil {
		logrus.Warnf("unable to determine runc version: %s", err.Error())
		return dockercompat.ComponentVersion{Name: "runc"}
	}

	var versionList = strings.Split(strings.TrimSpace(string(stdout)), "\n")
	firstLine := strings.Fields(versionList[0])
	if len(firstLine) != 3 {
		logrus.Errorf("unable to determine runc version, got: %s", firstLine)
		return dockercompat.ComponentVersion{Name: "runc"}
	}
	version := firstLine[2]

	details := map[string]string{}
	for _, detailsLine := range versionList[1:] {
		detail := strings.Split(detailsLine, ": ")
		if len(detail) != 2 {
			logrus.Warnf("unable to determine one of runc details, got: %s, %d", detail, len(detail))
			continue
		}
		details[detail[0]] = detail[1]
	}

	return dockercompat.ComponentVersion{
		Name:    "runc",
		Version: version,
		Details: details,
	}
}

//BlockIOWeight return whether Block IO weight is supported or not
func BlockIOWeight(cgroupManager string) bool {
	var info *dockercompat.Info
	info.CgroupVersion = CgroupsVersion()
	info.CgroupDriver = cgroupManager
	mobySysInfo := mobySysInfo(info)
	// blkio weight is not available on cgroup v1 since kernel 5.0.
	// On cgroup v2, blkio weight is implemented using io.weight
	return mobySysInfo.BlkioWeight
}
