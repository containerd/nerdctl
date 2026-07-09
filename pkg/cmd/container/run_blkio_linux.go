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
	"strconv"
	"strings"

	"github.com/docker/go-units"
	"github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/sys/unix"

	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/infoutil"
)

// WeightDevice is a structure that holds device:weight pair
type WeightDevice struct {
	Path   string
	Weight uint16
}

func (w *WeightDevice) String() string {
	return fmt.Sprintf("%s:%d", w.Path, w.Weight)
}

// ThrottleDevice is a structure that holds device:rate_per_second pair
type ThrottleDevice struct {
	Path string
	Rate uint64
}

func (t *ThrottleDevice) String() string {
	return fmt.Sprintf("%s:%d", t.Path, t.Rate)
}

func toOCIWeightDevices(weightDevices []*WeightDevice) ([]specs.LinuxWeightDevice, error) {
	var stat unix.Stat_t
	blkioWeightDevices := make([]specs.LinuxWeightDevice, 0, len(weightDevices))

	for _, weightDevice := range weightDevices {
		if err := unix.Stat(weightDevice.Path, &stat); err != nil {
			return nil, fmt.Errorf("failed to stat %s: %w", weightDevice.Path, err)
		}
		weight := weightDevice.Weight
		d := specs.LinuxWeightDevice{Weight: &weight}
		// The type is 32bit on mips.
		d.Major = int64(unix.Major(uint64(stat.Rdev))) //nolint: unconvert
		d.Minor = int64(unix.Minor(uint64(stat.Rdev))) //nolint: unconvert
		blkioWeightDevices = append(blkioWeightDevices, d)
	}

	return blkioWeightDevices, nil
}

func toOCIThrottleDevices(devs []*ThrottleDevice) ([]specs.LinuxThrottleDevice, error) {
	var stat unix.Stat_t
	throttleDevices := make([]specs.LinuxThrottleDevice, 0, len(devs))

	for _, d := range devs {
		if err := unix.Stat(d.Path, &stat); err != nil {
			return nil, fmt.Errorf("failed to stat %s: %w", d.Path, err)
		}
		d := specs.LinuxThrottleDevice{Rate: d.Rate}
		// the type is 32bit on mips
		d.Major = int64(unix.Major(uint64(stat.Rdev))) //nolint: unconvert
		d.Minor = int64(unix.Minor(uint64(stat.Rdev))) //nolint: unconvert
		throttleDevices = append(throttleDevices, d)
	}

	return throttleDevices, nil
}

func withBlkioWeight(blkioWeight uint16) oci.SpecOpts {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *specs.Spec) error {
		if s.Linux.Resources.BlockIO == nil {
			s.Linux.Resources.BlockIO = &specs.LinuxBlockIO{}
		}
		s.Linux.Resources.BlockIO.Weight = &blkioWeight
		return nil
	}
}

func withBlkioWeightDevice(weightDevices []specs.LinuxWeightDevice) oci.SpecOpts {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *specs.Spec) error {
		if s.Linux.Resources.BlockIO == nil {
			s.Linux.Resources.BlockIO = &specs.LinuxBlockIO{}
		}
		s.Linux.Resources.BlockIO.WeightDevice = weightDevices
		return nil
	}
}

func withBlkioReadBpsDevice(devices []specs.LinuxThrottleDevice) oci.SpecOpts {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *specs.Spec) error {
		if s.Linux.Resources.BlockIO == nil {
			s.Linux.Resources.BlockIO = &specs.LinuxBlockIO{}
		}
		s.Linux.Resources.BlockIO.ThrottleReadBpsDevice = devices
		return nil
	}
}

func withBlkioWriteBpsDevice(devices []specs.LinuxThrottleDevice) oci.SpecOpts {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *specs.Spec) error {
		if s.Linux.Resources.BlockIO == nil {
			s.Linux.Resources.BlockIO = &specs.LinuxBlockIO{}
		}
		s.Linux.Resources.BlockIO.ThrottleWriteBpsDevice = devices
		return nil
	}
}

func withBlkioReadIOPSDevice(devices []specs.LinuxThrottleDevice) oci.SpecOpts {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *specs.Spec) error {
		if s.Linux.Resources.BlockIO == nil {
			s.Linux.Resources.BlockIO = &specs.LinuxBlockIO{}
		}
		s.Linux.Resources.BlockIO.ThrottleReadIOPSDevice = devices
		return nil
	}
}

func withBlkioWriteIOPSDevice(devices []specs.LinuxThrottleDevice) oci.SpecOpts {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *specs.Spec) error {
		if s.Linux.Resources.BlockIO == nil {
			s.Linux.Resources.BlockIO = &specs.LinuxBlockIO{}
		}
		s.Linux.Resources.BlockIO.ThrottleWriteIOPSDevice = devices
		return nil
	}
}

func BlkioOCIOpts(options types.ContainerCreateOptions) ([]oci.SpecOpts, error) {
	var opts []oci.SpecOpts

	// Handle BlkioWeight
	if options.BlkioWeight != 0 {
		if !infoutil.BlockIOWeight(options.GOptions.CgroupManager) {
			// blkio weight is not available on cgroup v1 since kernel 5.0.
			// On cgroup v2, blkio weight is implemented using io.weight
			log.L.Warn("kernel support for cgroup blkio weight missing, weight discarded")
		} else {
			if options.BlkioWeight < 10 || options.BlkioWeight > 1000 {
				return nil, errors.New("range of blkio weight is from 10 to 1000")
			}
			opts = append(opts, withBlkioWeight(options.BlkioWeight))
		}
	}

	// Handle BlkioWeightDevice
	if len(options.BlkioWeightDevice) > 0 {
		if !infoutil.BlockIOWeightDevice(options.GOptions.CgroupManager) {
			// blkio weight device is not available on cgroup v1 since kernel 5.0.
			// On cgroup v2, blkio weight is implemented using io.weight
			log.L.Warn("kernel support for cgroup blkio weight device missing, weight device discarded")
		} else {
			weightDevices, err := validateWeightDevices(options.BlkioWeightDevice)
			if err != nil {
				return nil, fmt.Errorf("invalid weight device: %w", err)
			}
			linuxWeightDevices, err := toOCIWeightDevices(weightDevices)
			if err != nil {
				return nil, err
			}
			opts = append(opts, withBlkioWeightDevice(linuxWeightDevices))
		}
	}

	// Handle BlockIOReadBpsDevice
	if len(options.BlkioDeviceReadBps) > 0 {
		if !infoutil.BlockIOReadBpsDevice(options.GOptions.CgroupManager) {
			log.L.Warn("kernel support for cgroup blkio read bps device missing, read bps device discarded")
		} else {
			readBpsDevices, err := validateThrottleBpsDevices(options.BlkioDeviceReadBps)
			if err != nil {
				return nil, fmt.Errorf("invalid read bps device: %w", err)
			}
			throttleDevices, err := toOCIThrottleDevices(readBpsDevices)
			if err != nil {
				return nil, err
			}
			opts = append(opts, withBlkioReadBpsDevice(throttleDevices))
		}
	}

	// Handle BlockIOWriteBpsDevice
	if len(options.BlkioDeviceWriteBps) > 0 {
		if !infoutil.BlockIOWriteBpsDevice(options.GOptions.CgroupManager) {
			log.L.Warn("kernel support for cgroup blkio write bps device missing, write bps device discarded")
		} else {
			writeBpsDevices, err := validateThrottleBpsDevices(options.BlkioDeviceWriteBps)
			if err != nil {
				return nil, fmt.Errorf("invalid write bps device: %w", err)
			}
			throttleDevices, err := toOCIThrottleDevices(writeBpsDevices)
			if err != nil {
				return nil, err
			}
			opts = append(opts, withBlkioWriteBpsDevice(throttleDevices))
		}
	}

	// Handle BlockIOReadIopsDevice
	if len(options.BlkioDeviceReadIOps) > 0 {
		if !infoutil.BlockIOReadIOpsDevice(options.GOptions.CgroupManager) {
			log.L.Warn("kernel support for cgroup blkio read iops device missing, read iops device discarded")
		} else {
			readIopsDevices, err := validateThrottleIOpsDevices(options.BlkioDeviceReadIOps)
			if err != nil {
				return nil, fmt.Errorf("invalid read iops device: %w", err)
			}
			throttleDevices, err := toOCIThrottleDevices(readIopsDevices)
			if err != nil {
				return nil, err
			}
			opts = append(opts, withBlkioReadIOPSDevice(throttleDevices))
		}
	}

	// Handle BlockIOWriteIopsDevice
	if len(options.BlkioDeviceWriteIOps) > 0 {
		if !infoutil.BlockIOWriteIOpsDevice(options.GOptions.CgroupManager) {
			log.L.Warn("kernel support for cgroup blkio write iops device missing, write iops device discarded")
		} else {
			writeIopsDevices, err := validateThrottleIOpsDevices(options.BlkioDeviceWriteIOps)
			if err != nil {
				return nil, fmt.Errorf("invalid write iops device: %w", err)
			}
			throttleDevices, err := toOCIThrottleDevices(writeIopsDevices)
			if err != nil {
				return nil, err
			}
			opts = append(opts, withBlkioWriteIOPSDevice(throttleDevices))
		}
	}

	return opts, nil
}

// validateWeightDevices validates an array of device-weight strings
//
// from https://github.com/docker/cli/blob/master/opts/weightdevice.go#L15
func validateWeightDevices(vals []string) ([]*WeightDevice, error) {
	weightDevices := make([]*WeightDevice, 0, len(vals))
	for _, val := range vals {
		k, v, ok := strings.Cut(val, ":")
		if !ok || k == "" {
			return nil, fmt.Errorf("bad format: %s", val)
		}
		if !strings.HasPrefix(k, "/dev/") {
			return nil, fmt.Errorf("bad format for device path: %s", val)
		}
		weight, err := strconv.ParseUint(v, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid weight for device: %s", val)
		}
		if weight > 0 && (weight < 10 || weight > 1000) {
			return nil, fmt.Errorf("invalid weight for device: %s", val)
		}

		weightDevices = append(weightDevices, &WeightDevice{
			Path:   k,
			Weight: uint16(weight),
		})
	}
	return weightDevices, nil
}

// validateThrottleBpsDevices validates an array of device-rate strings for bytes per second
//
// from https://github.com/docker/cli/blob/master/opts/throttledevice.go#L16
func validateThrottleBpsDevices(vals []string) ([]*ThrottleDevice, error) {
	throttleDevices := make([]*ThrottleDevice, 0, len(vals))
	for _, val := range vals {
		k, v, ok := strings.Cut(val, ":")
		if !ok || k == "" {
			return nil, fmt.Errorf("bad format: %s", val)
		}

		if !strings.HasPrefix(k, "/dev/") {
			return nil, fmt.Errorf("bad format for device path: %s", val)
		}
		rate, err := units.RAMInBytes(v)
		if err != nil {
			return nil, fmt.Errorf("invalid rate for device: %s. The correct format is <device-path>:<number>[<unit>]. Number must be a positive integer. Unit is optional and can be kb, mb, or gb", val)
		}
		if rate < 0 {
			return nil, fmt.Errorf("invalid rate for device: %s. The correct format is <device-path>:<number>[<unit>]. Number must be a positive integer. Unit is optional and can be kb, mb, or gb", val)
		}

		throttleDevices = append(throttleDevices, &ThrottleDevice{
			Path: k,
			Rate: uint64(rate),
		})
	}
	return throttleDevices, nil
}

// validateThrottleIOpsDevices validates an array of device-rate strings for IO operations per second
//
// from https://github.com/docker/cli/blob/master/opts/throttledevice.go#L40
func validateThrottleIOpsDevices(vals []string) ([]*ThrottleDevice, error) {
	throttleDevices := make([]*ThrottleDevice, 0, len(vals))
	for _, val := range vals {
		k, v, ok := strings.Cut(val, ":")
		if !ok || k == "" {
			return nil, fmt.Errorf("bad format: %s", val)
		}

		if !strings.HasPrefix(k, "/dev/") {
			return nil, fmt.Errorf("bad format for device path: %s", val)
		}
		rate, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid rate for device: %s. The correct format is <device-path>:<number>. Number must be a positive integer", val)
		}

		throttleDevices = append(throttleDevices, &ThrottleDevice{
			Path: k,
			Rate: rate,
		})
	}
	return throttleDevices, nil
}
