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

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/introspection"
	ptypes "github.com/containerd/containerd/v2/pkg/protobuf/types"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/buildkitutil"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/v2/pkg/logging"
	"github.com/containerd/nerdctl/v2/pkg/version"
)

func NativeDaemonInfo(ctx context.Context, client *containerd.Client) (*native.DaemonInfo, error) {
	introService := client.IntrospectionService()
	plugins, err := introService.Plugins(ctx)
	if err != nil {
		return nil, err
	}
	server, err := introService.Server(ctx)
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
	daemonIntro, err := introService.Server(ctx)
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
	plugins, err := introService.Plugins(ctx)
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
		Version:   version.GetVersion(),
		GitCommit: version.GetRevision(),
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
	buildctlBinary, err := buildkitutil.BuildctlBinary()
	if err != nil {
		log.L.Warnf("unable to determine buildctl version: %s", err.Error())
		return dockercompat.ComponentVersion{Name: "buildctl"}
	}

	stdout, err := exec.Command(buildctlBinary, "--version").Output()
	if err != nil {
		log.L.Warnf("unable to determine buildctl version: %s", err.Error())
		return dockercompat.ComponentVersion{Name: "buildctl"}
	}

	v, err := parseBuildctlVersion(stdout)
	if err != nil {
		log.L.Warn(err)
		return dockercompat.ComponentVersion{Name: "buildctl"}
	}
	return *v
}

func parseBuildctlVersion(buildctlVersionStdout []byte) (*dockercompat.ComponentVersion, error) {
	fields := strings.Fields(strings.TrimSpace(string(buildctlVersionStdout)))
	var v *dockercompat.ComponentVersion
	switch len(fields) {
	case 4:
		v = &dockercompat.ComponentVersion{
			Name:    fields[0],
			Version: fields[2],
			Details: map[string]string{"GitCommit": fields[3]},
		}
	case 3:
		v = &dockercompat.ComponentVersion{
			Name:    fields[0],
			Version: fields[2],
		}
	default:
		return nil, fmt.Errorf("unable to determine buildctl version, got %q", string(buildctlVersionStdout))
	}
	if v.Name != "buildctl" {
		return nil, fmt.Errorf("unable to determine buildctl version, got %q", string(buildctlVersionStdout))
	}
	return v, nil
}

func runcVersion() dockercompat.ComponentVersion {
	stdout, err := exec.Command("runc", "--version").Output()
	if err != nil {
		log.L.Warnf("unable to determine runc version: %s", err.Error())
		return dockercompat.ComponentVersion{Name: "runc"}
	}
	v, err := parseRuncVersion(stdout)
	if err != nil {
		log.L.Warn(err)
		return dockercompat.ComponentVersion{Name: "runc"}
	}
	return *v
}

func parseRuncVersion(runcVersionStdout []byte) (*dockercompat.ComponentVersion, error) {
	var versionList = strings.Split(strings.TrimSpace(string(runcVersionStdout)), "\n")
	firstLine := strings.Fields(versionList[0])
	if len(firstLine) != 3 || firstLine[0] != "runc" {
		return nil, fmt.Errorf("unable to determine runc version, got: %s", string(runcVersionStdout))
	}
	version := firstLine[2]

	details := map[string]string{}
	for _, detailsLine := range versionList[1:] {
		detail := strings.SplitN(detailsLine, ":", 2)
		if len(detail) != 2 {
			log.L.Warnf("unable to determine one of runc details, got: %s, %d", detail, len(detail))
			continue
		}
		switch strings.TrimSpace(detail[0]) {
		case "commit":
			details["GitCommit"] = strings.TrimSpace(detail[1])
		}
	}

	return &dockercompat.ComponentVersion{
		Name:    "runc",
		Version: version,
		Details: details,
	}, nil
}

// BlockIOWeight return whether Block IO weight is supported or not
func BlockIOWeight(cgroupManager string) bool {
	var info dockercompat.Info
	info.CgroupVersion = CgroupsVersion()
	info.CgroupDriver = cgroupManager
	mobySysInfo := mobySysInfo(&info)
	// blkio weight is not available on cgroup v1 since kernel 5.0.
	// On cgroup v2, blkio weight is implemented using io.weight
	return mobySysInfo.BlkioWeight
}
