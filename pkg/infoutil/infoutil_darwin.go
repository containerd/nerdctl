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
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"

	"golang.org/x/sys/unix"
)

// UnameR returns `uname -r`
func UnameR() string {
	var utsname unix.Utsname
	if err := unix.Uname(&utsname); err != nil {
		// error is unlikely to happen
		return ""
	}
	var s string
	for _, f := range utsname.Release {
		if f == 0 {
			break
		}
		s += string(f)
	}
	return s
}

// UnameM returns `uname -m`
func UnameM() string {
	var utsname unix.Utsname
	if err := unix.Uname(&utsname); err != nil {
		// error is unlikely to happen
		return ""
	}
	var s string
	for _, f := range utsname.Machine {
		if f == 0 {
			break
		}
		s += string(f)
	}
	return s
}

const UnameO = "Darwin"

func DistroName() string {
	return ""
}

func CgroupsVersion() string {
	return ""
}

func fulfillPlatformInfo(info *dockercompat.Info) {
	// unimplemented
}
