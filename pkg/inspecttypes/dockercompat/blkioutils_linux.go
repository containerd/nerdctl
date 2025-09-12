//go:build linux

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
	"fmt"
	"os"

	"github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/sys/unix"
)

func toDockerCompatWeightDevices(weightDevices []specs.LinuxWeightDevice) ([]WeightDevice, error) {
	majorMinorToPathMap, err := getDeviceMajorMinorToPathMap()
	if err != nil {
		return nil, fmt.Errorf("failed to query device paths from major/minor numbers: %w", err)
	}

	devices := []WeightDevice{}
	for _, weightDevice := range weightDevices {
		key := fmt.Sprintf("%d:%d", weightDevice.Major, weightDevice.Minor)
		if _, ok := majorMinorToPathMap[key]; ok {
			devices = append(devices, WeightDevice{
				Path:   majorMinorToPathMap[key],
				Weight: *weightDevice.Weight,
			})
		}
	}
	return devices, nil
}

func toDockerCompatThrottleDevices(throttleDevices []specs.LinuxThrottleDevice) ([]ThrottleDevice, error) {
	majorMinorToPathMap, err := getDeviceMajorMinorToPathMap()
	if err != nil {
		return nil, fmt.Errorf("failed to query device paths from major/minor numbers: %w", err)
	}

	devices := []ThrottleDevice{}
	for _, throttleDevice := range throttleDevices {
		key := fmt.Sprintf("%d:%d", throttleDevice.Major, throttleDevice.Minor)
		if _, ok := majorMinorToPathMap[key]; ok {
			devices = append(devices, ThrottleDevice{
				Path: majorMinorToPathMap[key],
				Rate: throttleDevice.Rate,
			})
		}
	}
	return devices, nil
}

func getDeviceMajorMinorToPathMap() (map[string]string, error) {
	devDir := "/dev"
	entries, err := os.ReadDir(devDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", devDir, err)
	}

	majorMinorToPathMap := make(map[string]string)
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		devicePath := fmt.Sprintf("%s/%s", devDir, ent.Name())
		osStat, err := os.Stat(devicePath)
		if err != nil {
			return nil, fmt.Errorf("failed to stat %s: %w", devicePath, err)
		}
		// skip char devices
		if osStat.Mode()&os.ModeCharDevice != 0 {
			continue
		}
		var unixStat unix.Stat_t
		if err := unix.Stat(devicePath, &unixStat); err != nil {
			return nil, fmt.Errorf("failed to stat %s: %w", devicePath, err)
		}
		major := int64(unix.Major(uint64(unixStat.Rdev))) //nolint: unconvert
		minor := int64(unix.Minor(uint64(unixStat.Rdev))) //nolint: unconvert
		key := fmt.Sprintf("%d:%d", major, minor)
		majorMinorToPathMap[key] = devicePath
	}
	return majorMinorToPathMap, nil
}
