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

package test

import (
	"fmt"
	"os/exec"
	"runtime"
)

func Binary(name string) *Requirement {
	return &Requirement{
		Check: func(data Data, helpers Helpers) (bool, string) {
			mess := fmt.Sprintf("executable %q has been found in PATH", name)
			ret := true
			if _, err := exec.LookPath(name); err != nil {
				ret = false
				mess = fmt.Sprintf("executable %q doesn't exist in PATH", name)
			}

			return ret, mess
		},
	}
}

func OS(os string) *Requirement {
	return &Requirement{
		Check: func(data Data, helpers Helpers) (bool, string) {
			mess := fmt.Sprintf("current operating system is %q", runtime.GOOS)
			ret := true
			if runtime.GOOS != os {
				ret = false
			}

			return ret, mess
		},
	}
}

func Arch(arch string) *Requirement {
	return &Requirement{
		Check: func(data Data, helpers Helpers) (bool, string) {
			mess := fmt.Sprintf("current architecture is %q", runtime.GOARCH)
			ret := true
			if runtime.GOARCH != arch {
				ret = false
			}

			return ret, mess
		},
	}
}

var Amd64 = Arch("amd64")
var Arm64 = Arch("arm64")
var Windows = OS("windows")
var Linux = OS("linux")
var Darwin = OS("darwin")

// NOTE: Not will always lose setups and cleanups...

func Not(requirement *Requirement) *Requirement {
	return &Requirement{
		Check: func(data Data, helpers Helpers) (bool, string) {
			ret, mess := requirement.Check(data, helpers)
			return !ret, mess
		},
	}
}

func Require(requirements ...*Requirement) *Requirement {
	return &Requirement{
		Check: func(data Data, helpers Helpers) (bool, string) {
			ret := true
			mess := ""
			var subMess string
			for _, requirement := range requirements {
				ret, subMess = requirement.Check(data, helpers)
				mess += "\n" + subMess
				if !ret {
					return ret, mess
				}
			}
			return ret, mess
		},
		Setup: func(data Data, helpers Helpers) {
			for _, requirement := range requirements {
				if requirement.Setup != nil {
					requirement.Setup(data, helpers)
				}
			}
		},
		Cleanup: func(data Data, helpers Helpers) {
			for _, requirement := range requirements {
				if requirement.Cleanup != nil {
					requirement.Cleanup(data, helpers)
				}
			}
		},
	}
}
