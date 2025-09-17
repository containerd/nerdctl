//go:build !linux

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

	"github.com/opencontainers/runtime-spec/specs-go"
)

func toDockerCompatWeightDevices(weightDevices []specs.LinuxWeightDevice) ([]WeightDevice, error) {
	return nil, fmt.Errorf("block device weight controls are not supported on this platform")
}

func toDockerCompatThrottleDevices(throttleDevices []specs.LinuxThrottleDevice) ([]ThrottleDevice, error) {
	return nil, fmt.Errorf("block device throttling is not supported on this platform")
}
