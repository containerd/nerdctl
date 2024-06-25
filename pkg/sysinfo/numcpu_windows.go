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
   Portions from https://github.com/moby/moby/blob/cff4f20c44a3a7c882ed73934dec6a77246c6323/pkg/sysinfo/numcpu_windows.go
   Copyright (C) Docker/Moby authors.
   Licensed under the Apache License, Version 2.0
   NOTICE: https://github.com/moby/moby/blob/cff4f20c44a3a7c882ed73934dec6a77246c6323/NOTICE
*/

package sysinfo // import "github.com/docker/docker/pkg/sysinfo"

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32               = windows.NewLazySystemDLL("kernel32.dll")
	getCurrentProcess      = kernel32.NewProc("GetCurrentProcess")
	getProcessAffinityMask = kernel32.NewProc("GetProcessAffinityMask")
)

// Returns bit count of 1, used by NumCPU
func popcnt(x uint64) (n byte) {
	x -= (x >> 1) & 0x5555555555555555
	x = (x>>2)&0x3333333333333333 + x&0x3333333333333333
	x += x >> 4
	x &= 0x0f0f0f0f0f0f0f0f
	x *= 0x0101010101010101
	return byte(x >> 56)
}

func numCPU() int {
	// Gets the affinity mask for a process
	var mask, sysmask uintptr
	currentProcess, _, _ := getCurrentProcess.Call()
	ret, _, _ := getProcessAffinityMask.Call(currentProcess, uintptr(unsafe.Pointer(&mask)), uintptr(unsafe.Pointer(&sysmask)))
	if ret == 0 {
		return 0
	}
	// For every available thread a bit is set in the mask.
	ncpu := int(popcnt(uint64(mask)))
	return ncpu
}
