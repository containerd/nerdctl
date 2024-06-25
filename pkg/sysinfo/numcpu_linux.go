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
   Portions from https://github.com/moby/moby/blob/cff4f20c44a3a7c882ed73934dec6a77246c6323/pkg/sysinfo/numcpu_linux.go
   Copyright (C) Docker/Moby authors.
   Licensed under the Apache License, Version 2.0
   NOTICE: https://github.com/moby/moby/blob/cff4f20c44a3a7c882ed73934dec6a77246c6323/NOTICE
*/

package sysinfo // import "github.com/docker/docker/pkg/sysinfo"

import "golang.org/x/sys/unix"

// numCPU queries the system for the count of threads available
// for use to this process.
//
// Returns 0 on errors. Use |runtime.NumCPU| in that case.
func numCPU() int {
	// Gets the affinity mask for a process: The very one invoking this function.
	var mask unix.CPUSet
	err := unix.SchedGetaffinity(0, &mask)
	if err != nil {
		return 0
	}
	return mask.Count()
}
