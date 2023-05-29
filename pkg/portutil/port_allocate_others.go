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

package portutil

import "fmt"

func portAllocate(protocol string, ip string, count uint64) (uint64, uint64, error) {
	return 0, 0, fmt.Errorf("auto port allocate are not support Non-Linux platform yet")
}

func getUsedPorts(ip string, protocol string) (map[uint64]bool, error) {
	return nil, nil
}
