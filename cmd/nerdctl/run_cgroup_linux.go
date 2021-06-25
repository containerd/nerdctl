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

package main

import (
	"path/filepath"
	"strings"

	"github.com/containerd/containerd/oci"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/docker/go-units"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

func generateCgroupOpts(clicontext *cli.Context, id string) ([]oci.SpecOpts, error) {
	if clicontext.String("cgroup-manager") == "none" {
		if !rootlessutil.IsRootless() {
			return nil, errors.New("cgroup-manager \"none\" is only supported for rootless")
		}
		if clicontext.Float64("cpus") > 0.0 || clicontext.String("memory") != "" ||
			clicontext.Int("pids-limit") > 0 {
			logrus.Warn("cgroup manager is set to \"none\", discarding resource limit requests. " +
				"(Hint: enable cgroup v2 with systemd: https://rootlesscontaine.rs/getting-started/common/cgroup2/)")
		}
		return []oci.SpecOpts{oci.WithCgroup("")}, nil
	}

	var opts []oci.SpecOpts // nolint: prealloc

	if clicontext.String("cgroup-manager") == "systemd" {
		slice := "system.slice"
		if rootlessutil.IsRootlessChild() {
			slice = "user.slice"
		}
		//  "slice:prefix:name"
		cg := slice + ":nerdctl:" + id
		opts = append(opts, oci.WithCgroup(cg))
	}

	// cpus: from https://github.com/containerd/containerd/blob/v1.4.3/cmd/ctr/commands/run/run_unix.go#L187-L193
	if cpus := clicontext.Float64("cpus"); cpus > 0.0 {
		var (
			period = uint64(100000)
			quota  = int64(cpus * 100000.0)
		)
		opts = append(opts, oci.WithCPUCFS(quota, period))
	}

	if shares := clicontext.Int("cpu-shares"); shares != 0 {
		var (
			shares = uint64(shares)
		)
		opts = append(opts, oci.WithCPUShares(shares))
	}

	if cpuset := clicontext.String("cpuset-cpus"); cpuset != "" {
		opts = append(opts, oci.WithCPUs(cpuset))
	}

	if memStr := clicontext.String("memory"); memStr != "" {
		mem64, err := units.RAMInBytes(memStr)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse memory bytes %q", memStr)
		}
		opts = append(opts, oci.WithMemoryLimit(uint64(mem64)))
	}

	if pidsLimit := clicontext.Int("pids-limit"); pidsLimit > 0 {
		opts = append(opts, oci.WithPidsLimit(int64(pidsLimit)))
	}

	switch cgroupns := clicontext.String("cgroupns"); cgroupns {
	case "private":
		ns := specs.LinuxNamespace{
			Type: specs.CgroupNamespace,
		}
		opts = append(opts, oci.WithLinuxNamespace(ns))
	case "host":
		opts = append(opts, oci.WithHostNamespace(specs.CgroupNamespace))
	default:
		return nil, errors.Errorf("unknown cgroupns mode %q", cgroupns)
	}

	for _, f := range clicontext.StringSlice("device") {
		devPath, mode, err := parseDevice(f)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse device %q", f)
		}
		opts = append(opts, oci.WithLinuxDevice(devPath, mode))
	}
	return opts, nil
}

func parseDevice(s string) (hostDevPath string, mode string, err error) {
	mode = "rwm"
	split := strings.Split(s, ":")
	var containerDevPath string
	switch len(split) {
	case 1: // e.g. "/dev/sda1"
		hostDevPath = split[0]
		containerDevPath = hostDevPath
	case 2: // e.g., "/dev/sda1:rwm", or "/dev/sda1:/dev/sda1
		hostDevPath = split[0]
		if !strings.Contains(split[1], "/") {
			containerDevPath = hostDevPath
			mode = split[1]
		} else {
			containerDevPath = split[1]
		}
	case 3: // e.g., "/dev/sda1:/dev/sda1:rwm"
		hostDevPath = split[0]
		containerDevPath = split[1]
		mode = split[2]
	default:
		return "", "", errors.New("too many `:` symbols")
	}

	if containerDevPath != hostDevPath {
		return "", "", errors.New("changing the path inside the container is not supported yet")
	}

	if !filepath.IsAbs(hostDevPath) {
		return "", "", errors.Errorf("%q is not an absolute path", hostDevPath)
	}

	if err := validateDeviceMode(mode); err != nil {
		return "", "", err
	}
	return hostDevPath, mode, nil
}

func validateDeviceMode(mode string) error {
	for _, r := range mode {
		switch r {
		case 'r', 'w', 'm':
		default:
			return errors.Errorf("invalid mode %q: unexpected rune %v", mode, r)
		}
	}
	return nil
}
