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

func MakeRequirement(fn func(data Data) (bool, string)) Requirement {
	return func(data Data, t *testing.T) (bool, string) {
		ret, mess := fn(data)

		if t != nil && !ret {
			t.Helper()
			t.Skipf("Test skipped as %s", mess)
		}

		return ret, mess
	}
}

func Binary(name string) Requirement {
	return MakeRequirement(func(data Data) (ret bool, mess string) {
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
	return MakeRequirement(func(data Data) (ret bool, mess string) {
		mess = fmt.Sprintf("current operating is %q", runtime.GOOS)
		ret = true
		if runtime.GOOS != os {
			ret = false
		}

		return ret, mess
	})
}

var Windows = MakeRequirement(func(data Data) (ret bool, mess string) {
	ret = runtime.GOOS == "windows"
	if ret {
		mess = "operating system is Windows"
	} else {
		mess = "operating system is not Windows"
	}
	return ret, mess
})

var Linux = MakeRequirement(func(data Data) (ret bool, mess string) {
	ret = runtime.GOOS == "linux"
	if ret {
		mess = "operating system is Linux"
	} else {
		mess = "operating system is not Linux"
	}
	return ret, mess
})

var Darwin = MakeRequirement(func(data Data) (ret bool, mess string) {
	ret = runtime.GOOS == "darwin"
	if ret {
		mess = "operating system is Darwin"
	} else {
		mess = "operating system is not Darwin"
	}
	return ret, mess
})

func Not(requirement Requirement) Requirement {
	return MakeRequirement(func(data Data) (ret bool, mess string) {
		b, mess := requirement(data, nil)
		return !b, mess
	})
}

func Require(thing ...Requirement) Requirement {
	return func(data Data, t *testing.T) (ret bool, mess string) {
		for _, th := range thing {
			b, m := th(data, nil)
			if !b {
				if t != nil {
					t.Helper()
					t.Skipf("Test skipped as %s", m)
				}
				return false, ""
			}
		}
		return true, ""
	}
}
