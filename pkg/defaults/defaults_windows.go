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

package defaults

import (
	"fmt"
)

const AppArmorProfileName = ""

func DataRoot() string {
	return "C:\\ProgramData\\containerd\\root"
}

func CNIPath() string {
	return "C:\\Program Files\\containerd\\cni\\bin"
}

func CNINetConfPath() string {
	return "C:\\Program Files\\containerd\\cni\\conf"
}

func BuildKitHost() string {
	return fmt.Sprint("\\\\.\\pipe\\buildkit")
}

func IsSystemdAvailable() bool {
	return false
}

func CgroupManager() string {
	return ""
}

func CgroupsVersion() string {
	return ""
}

func CgroupnsMode() string {
	return ""
}
