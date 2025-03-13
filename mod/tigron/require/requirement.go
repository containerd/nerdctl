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

package require

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/containerd/nerdctl/mod/tigron/test"
)

func Binary(name string) *test.Requirement {
	return &test.Requirement{
		Check: func(_ test.Data, _ test.Helpers) (bool, string) {
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

func OS(os string) *test.Requirement {
	return &test.Requirement{
		Check: func(_ test.Data, _ test.Helpers) (bool, string) {
			mess := fmt.Sprintf("current operating system is %q", runtime.GOOS)
			ret := true
			if runtime.GOOS != os {
				ret = false
			}

			return ret, mess
		},
	}
}

func Arch(arch string) *test.Requirement {
	return &test.Requirement{
		Check: func(_ test.Data, _ test.Helpers) (bool, string) {
			mess := fmt.Sprintf("current architecture is %q", runtime.GOARCH)
			ret := true
			if runtime.GOARCH != arch {
				ret = false
			}

			return ret, mess
		},
	}
}

//nolint:gochecknoglobals
var (
	Amd64   = Arch("amd64")
	Arm64   = Arch("arm64")
	Windows = OS("windows")
	Linux   = OS("linux")
	Darwin  = OS("darwin")
)

// NOTE: Not will always ignore any setup and cleanup inside the wrapped requirement.

func Not(requirement *test.Requirement) *test.Requirement {
	return &test.Requirement{
		Check: func(data test.Data, helpers test.Helpers) (bool, string) {
			ret, mess := requirement.Check(data, helpers)

			return !ret, mess
		},
	}
}

func All(requirements ...*test.Requirement) *test.Requirement {
	return &test.Requirement{
		Check: func(data test.Data, helpers test.Helpers) (bool, string) {
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
		Setup: func(data test.Data, helpers test.Helpers) {
			for _, requirement := range requirements {
				if requirement.Setup != nil {
					requirement.Setup(data, helpers)
				}
			}
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			for _, requirement := range requirements {
				if requirement.Cleanup != nil {
					requirement.Cleanup(data, helpers)
				}
			}
		},
	}
}
