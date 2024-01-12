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

package container

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/infoutil"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/docker/go-units"
	"github.com/opencontainers/runtime-spec/specs-go"
)

type customMemoryOptions struct {
	MemoryReservation *int64
	MemorySwappiness  *uint64
	disableOOMKiller  *bool
}

func generateCgroupOpts(id string, options types.ContainerCreateOptions) ([]oci.SpecOpts, error) {
	if options.KernelMemory != "" {
		log.L.Warnf("The --kernel-memory flag is no longer supported. This flag is a noop.")
	}

	if options.Memory == "" && options.OomKillDisable {
		log.L.Warn("Disabling the OOM killer on containers without setting a '-m/--memory' limit may be dangerous.")
	}

	if options.GOptions.CgroupManager == "none" {
		if !rootlessutil.IsRootless() {
			return nil, errors.New(`cgroup-manager "none" is only supported for rootless`)
		}

		if options.CPUs > 0.0 || options.Memory != "" || options.MemorySwap != "" || options.PidsLimit > 0 {
			log.L.Warn(`cgroup manager is set to "none", discarding resource limit requests. ` +
				"(Hint: enable cgroup v2 with systemd: https://rootlesscontaine.rs/getting-started/common/cgroup2/)")
		}
		if options.CgroupParent != "" {
			log.L.Warnf(`cgroup manager is set to "none", ignoring cgroup parent %q`+
				"(Hint: enable cgroup v2 with systemd: https://rootlesscontaine.rs/getting-started/common/cgroup2/)", options.CgroupParent)
		}
		return []oci.SpecOpts{oci.WithCgroup("")}, nil
	}

	var opts []oci.SpecOpts // nolint: prealloc
	path, err := generateCgroupPath(id, options.GOptions.CgroupManager, options.CgroupParent)
	if err != nil {
		return nil, err
	}
	if path != "" {
		opts = append(opts, oci.WithCgroup(path))
	}

	// cpus: from https://github.com/containerd/containerd/blob/v1.4.3/cmd/ctr/commands/run/run_unix.go#L187-L193
	if options.CPUs > 0.0 {
		var (
			period = uint64(100000)
			quota  = int64(options.CPUs * 100000.0)
		)
		opts = append(opts, oci.WithCPUCFS(quota, period))
	}

	if options.CPUShares != 0 {
		opts = append(opts, oci.WithCPUShares(options.CPUShares))
	}

	if options.CPUSetCPUs != "" {
		opts = append(opts, oci.WithCPUs(options.CPUSetCPUs))
	}
	if options.CPUQuota != -1 || options.CPUPeriod != 0 {
		if options.CPUs > 0.0 {
			return nil, errors.New("cpus and quota/period should be used separately")
		}
		opts = append(opts, oci.WithCPUCFS(options.CPUQuota, options.CPUPeriod))
	}
	if options.CPUSetMems != "" {
		opts = append(opts, oci.WithCPUsMems(options.CPUSetMems))
	}

	var mem64 int64
	if options.Memory != "" {
		mem64, err = units.RAMInBytes(options.Memory)
		if err != nil {
			return nil, fmt.Errorf("failed to parse memory bytes %q: %w", options.Memory, err)
		}
		opts = append(opts, oci.WithMemoryLimit(uint64(mem64)))
	}

	var memReserve64 int64
	if options.MemoryReservation != "" {
		memReserve64, err = units.RAMInBytes(options.MemoryReservation)
		if err != nil {
			return nil, fmt.Errorf("failed to parse memory bytes %q: %w", options.MemoryReservation, err)
		}
	}
	var memSwap64 int64
	if options.MemorySwap != "" {
		if options.MemorySwap == "-1" {
			memSwap64 = -1
		} else {
			memSwap64, err = units.RAMInBytes(options.MemorySwap)
			if err != nil {
				return nil, fmt.Errorf("failed to parse memory-swap bytes %q: %w", options.MemorySwap, err)
			}
			if mem64 > 0 && memSwap64 > 0 && memSwap64 < mem64 {
				return nil, fmt.Errorf("minimum memoryswap limit should be larger than memory limit, see usage")
			}
		}
	} else {
		// if `--memory-swap` is unset, the container can use as much swap as the `--memory` setting.
		memSwap64 = mem64 * 2
	}
	if memSwap64 == 0 {
		// if --memory-swap is set to 0, the setting is ignored, and the value is treated as unset.
		memSwap64 = mem64 * 2
	}
	if memSwap64 != 0 {
		opts = append(opts, oci.WithMemorySwap(memSwap64))
	}
	if mem64 > 0 && memReserve64 > 0 && mem64 < memReserve64 {
		return nil, fmt.Errorf("minimum memory limit can not be less than memory reservation limit, see usage")
	}
	if options.MemorySwappiness64 > 100 || options.MemorySwappiness64 < -1 {
		return nil, fmt.Errorf("invalid value: %v, valid memory swappiness range is 0-100", options.MemorySwappiness64)
	}

	var customMemRes customMemoryOptions
	if memReserve64 >= 0 && options.MemoryReservationChanged {
		customMemRes.MemoryReservation = &memReserve64
	}
	if options.MemorySwappiness64 >= 0 && options.MemorySwappiness64Changed {
		memSwapinessUint64 := uint64(options.MemorySwappiness64)
		customMemRes.MemorySwappiness = &memSwapinessUint64
	}
	if options.OomKillDisable {
		customMemRes.disableOOMKiller = &options.OomKillDisable
	}
	opts = append(opts, withCustomMemoryResources(customMemRes))

	if options.PidsLimit > 0 {
		opts = append(opts, oci.WithPidsLimit(options.PidsLimit))
	}

	if len(options.CgroupConf) > 0 && infoutil.CgroupsVersion() == "1" {
		return nil, errors.New("cannot use --cgroup-conf without cgroup v2")
	}

	unifieds := make(map[string]string)
	for _, unified := range options.CgroupConf {
		splitUnified := strings.SplitN(unified, "=", 2)
		if len(splitUnified) < 2 {
			return nil, errors.New("--cgroup-conf must be formatted KEY=VALUE")
		}
		unifieds[splitUnified[0]] = splitUnified[1]
	}
	opts = append(opts, withUnified(unifieds))

	if options.BlkioWeight != 0 && !infoutil.BlockIOWeight(options.GOptions.CgroupManager) {
		log.L.Warn("kernel support for cgroup blkio weight missing, weight discarded")
		options.BlkioWeight = 0
	}
	if options.BlkioWeight > 0 && options.BlkioWeight < 10 || options.BlkioWeight > 1000 {
		return nil, errors.New("range of blkio weight is from 10 to 1000")
	}
	opts = append(opts, withBlkioWeight(options.BlkioWeight))

	switch options.Cgroupns {
	case "private":
		ns := specs.LinuxNamespace{
			Type: specs.CgroupNamespace,
		}
		opts = append(opts, oci.WithLinuxNamespace(ns))
	case "host":
		opts = append(opts, oci.WithHostNamespace(specs.CgroupNamespace))
	default:
		return nil, fmt.Errorf("unknown cgroupns mode %q", options.Cgroupns)
	}

	for _, f := range options.Device {
		devPath, mode, err := ParseDevice(f)
		if err != nil {
			return nil, fmt.Errorf("failed to parse device %q: %w", f, err)
		}
		opts = append(opts, oci.WithLinuxDevice(devPath, mode))
	}
	return opts, nil
}

func generateCgroupPath(id, cgroupManager, cgroupParent string) (string, error) {
	var (
		path         string
		usingSystemd = cgroupManager == "systemd"
		slice        = "system.slice"
		scopePrefix  = ":nerdctl:"
	)
	if rootlessutil.IsRootlessChild() {
		slice = "user.slice"
	}

	if cgroupParent == "" {
		if usingSystemd {
			// "slice:prefix:name"
			path = slice + scopePrefix + id
		}
		// Nothing to do for the non-systemd case if a parent wasn't supplied,
		// containerd already sets a default cgroup path as /<namespace>/<containerID>
		return path, nil
	}

	// If the user asked for a cgroup parent, we will use systemd,
	// Docker uses the following:
	// parent + prefix (in our case, nerdctl) + containerID.
	//
	// In the non systemd case, it's just /parent/containerID
	if usingSystemd {
		if len(cgroupParent) <= 6 || !strings.HasSuffix(cgroupParent, ".slice") {
			return "", errors.New(`cgroup-parent for systemd cgroup should be a valid slice named as "xxx.slice"`)
		}
		path = cgroupParent + scopePrefix + id
	} else {
		path = filepath.Join(cgroupParent, id)
	}

	return path, nil
}

// ParseDevice parses the give device string into hostDevPath and mode(defaults: "rwm").
func ParseDevice(s string) (hostDevPath string, mode string, err error) {
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
		return "", "", fmt.Errorf("%q is not an absolute path", hostDevPath)
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
			return fmt.Errorf("invalid mode %q: unexpected rune %v", mode, r)
		}
	}
	return nil
}

func withUnified(unified map[string]string) oci.SpecOpts {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) (err error) {
		if unified == nil {
			return nil
		}
		s.Linux.Resources.Unified = make(map[string]string)
		for k, v := range unified {
			s.Linux.Resources.Unified[k] = v
		}
		return nil
	}
}

func withBlkioWeight(blkioWeight uint16) oci.SpecOpts {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
		if blkioWeight == 0 {
			return nil
		}
		s.Linux.Resources.BlockIO = &specs.LinuxBlockIO{Weight: &blkioWeight}
		return nil
	}
}

func withCustomMemoryResources(memoryOptions customMemoryOptions) oci.SpecOpts {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
		if s.Linux != nil {
			if s.Linux.Resources == nil {
				s.Linux.Resources = &specs.LinuxResources{}
			}
			if s.Linux.Resources.Memory == nil {
				s.Linux.Resources.Memory = &specs.LinuxMemory{}
			}
			if memoryOptions.disableOOMKiller != nil {
				s.Linux.Resources.Memory.DisableOOMKiller = memoryOptions.disableOOMKiller
			}
			if memoryOptions.MemorySwappiness != nil {
				s.Linux.Resources.Memory.Swappiness = memoryOptions.MemorySwappiness
			}
			if memoryOptions.MemoryReservation != nil {
				s.Linux.Resources.Memory.Reservation = memoryOptions.MemoryReservation
			}
		}
		return nil
	}
}
