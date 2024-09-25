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
	"testing"
)

func MakeRequirement(fn func(data Data, t *testing.T) (bool, string)) Requirement {
	return func(data Data, skip bool, t *testing.T) (bool, string) {
		ret, mess := fn(data, t)

		if skip && !ret {
			t.Helper()
			t.Skipf("Test skipped as %s", mess)
		}

		return ret, mess
	}
}

func Binary(name string) Requirement {
	return MakeRequirement(func(data Data, t *testing.T) (ret bool, mess string) {
		mess = fmt.Sprintf("executable %q has been found in PATH", name)
		ret = true
		if _, err := exec.LookPath(name); err != nil {
			ret = false
			mess = fmt.Sprintf("executable %q doesn't exist in PATH", name)
		}

		return ret, mess
	})
}

func OS(os string) Requirement {
	return MakeRequirement(func(data Data, t *testing.T) (ret bool, mess string) {
		mess = fmt.Sprintf("current operating system is %q", runtime.GOOS)
		ret = true
		if runtime.GOOS != os {
			ret = false
		}

		return ret, mess
	})
}

func Arch(arch string) Requirement {
	return MakeRequirement(func(data Data, t *testing.T) (ret bool, mess string) {
		mess = fmt.Sprintf("current architecture is %q", runtime.GOARCH)
		ret = true
		if runtime.GOARCH != arch {
			ret = false
		}

		return ret, mess
	})
}

var Amd64 = Arch("amd64")
var Arm64 = Arch("arm64")
var Windows = OS("windows")
var Linux = OS("linux")
var Darwin = OS("darwin")

func Not(requirement Requirement) Requirement {
	return MakeRequirement(func(data Data, t *testing.T) (ret bool, mess string) {
		b, mess := requirement(data, false, t)

		return !b, mess
	})
}

func Require(thing ...Requirement) Requirement {
	return func(data Data, skip bool, t *testing.T) (ret bool, mess string) {
		for _, th := range thing {
			b, m := th(data, false, t)
			if !b {
				if skip {
					t.Helper()
					t.Skipf("Test skipped as %s", m)
				}
				return false, ""
			}
		}
		return true, ""
	}
}
