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
   Portions from https://github.com/moby/moby/blob/v20.10.1/api/types/blkiodev/blkio.go
   Copyright (C) Docker/Moby authors.
   Licensed under the Apache License, Version 2.0
   NOTICE: https://github.com/moby/moby/blob/v20.10.1/NOTICE
*/

package dockercompat

import (
	"fmt"

	"github.com/opencontainers/runtime-spec/specs-go"
)

type BlkioSettings struct {
	BlkioWeight          uint16 // Block IO weight (relative weight vs. other containers)
	BlkioWeightDevice    []*WeightDevice
	BlkioDeviceReadBps   []*ThrottleDevice
	BlkioDeviceWriteBps  []*ThrottleDevice
	BlkioDeviceReadIOps  []*ThrottleDevice
	BlkioDeviceWriteIOps []*ThrottleDevice
}

// From https://github.com/moby/moby/blob/v20.10.1/api/types/blkiodev/blkio.go
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

func getBlkioSettingsFromSpec(spec *specs.Spec, hostConfig *HostConfig) error {
	if spec == nil {
		return fmt.Errorf("spec cannot be nil")
	}
	if hostConfig == nil {
		return fmt.Errorf("hostConfig cannot be nil")
	}

	// Initialize empty arrays by default
	hostConfig.BlkioSettings = getDefaultBlkioSettings()

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
		hostConfig.BlkioWeightDevice = make([]*WeightDevice, len(blockIO.WeightDevice))
		dockerCompatWeightDevices, err := toDockerCompatWeightDevices(blockIO.WeightDevice)
		if err != nil {
			return fmt.Errorf("failed to convert weight devices to dockercompat format: %w", err)
		}
		for i, dev := range dockerCompatWeightDevices {
			hostConfig.BlkioWeightDevice[i] = &dev
		}
	}

	// Set throttle devices for read BPS
	if len(blockIO.ThrottleReadBpsDevice) > 0 {
		hostConfig.BlkioDeviceReadBps = make([]*ThrottleDevice, len(blockIO.ThrottleReadBpsDevice))
		dockerCompatThrottleDevices, err := toDockerCompatThrottleDevices(blockIO.ThrottleReadBpsDevice)
		if err != nil {
			return fmt.Errorf("failed to convert throttle devices to dockercompat format: %w", err)
		}
		for i, dev := range dockerCompatThrottleDevices {
			hostConfig.BlkioDeviceReadBps[i] = &dev
		}
	}

	// Set throttle devices for write BPS
	if len(blockIO.ThrottleWriteBpsDevice) > 0 {
		hostConfig.BlkioDeviceWriteBps = make([]*ThrottleDevice, len(blockIO.ThrottleWriteBpsDevice))
		dockerCompatThrottleDevices, err := toDockerCompatThrottleDevices(blockIO.ThrottleWriteBpsDevice)
		if err != nil {
			return fmt.Errorf("failed to convert throttle devices to dockercompat format: %w", err)
		}
		for i, dev := range dockerCompatThrottleDevices {
			hostConfig.BlkioDeviceWriteBps[i] = &dev
		}
	}

	// Set throttle devices for read IOPs
	if len(blockIO.ThrottleReadIOPSDevice) > 0 {
		hostConfig.BlkioDeviceReadIOps = make([]*ThrottleDevice, len(blockIO.ThrottleReadIOPSDevice))
		dockerCompatThrottleDevices, err := toDockerCompatThrottleDevices(blockIO.ThrottleReadIOPSDevice)
		if err != nil {
			return fmt.Errorf("failed to convert throttle devices to dockercompat format: %w", err)
		}
		for i, dev := range dockerCompatThrottleDevices {
			hostConfig.BlkioDeviceReadIOps[i] = &dev
		}
	}

	// Set throttle devices for write IOPs
	if len(blockIO.ThrottleWriteIOPSDevice) > 0 {
		hostConfig.BlkioDeviceWriteIOps = make([]*ThrottleDevice, len(blockIO.ThrottleWriteIOPSDevice))
		dockerCompatThrottleDevices, err := toDockerCompatThrottleDevices(blockIO.ThrottleWriteIOPSDevice)
		if err != nil {
			return fmt.Errorf("failed to convert throttle devices to dockercompat format: %w", err)
		}
		for i, dev := range dockerCompatThrottleDevices {
			hostConfig.BlkioDeviceWriteIOps[i] = &dev
		}
	}
	return nil
}

func getDefaultBlkioSettings() BlkioSettings {
	return BlkioSettings{
		BlkioWeight:          0,
		BlkioWeightDevice:    make([]*WeightDevice, 0),
		BlkioDeviceReadBps:   make([]*ThrottleDevice, 0),
		BlkioDeviceWriteBps:  make([]*ThrottleDevice, 0),
		BlkioDeviceReadIOps:  make([]*ThrottleDevice, 0),
		BlkioDeviceWriteIOps: make([]*ThrottleDevice, 0),
	}
}
