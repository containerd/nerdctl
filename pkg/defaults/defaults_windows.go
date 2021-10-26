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
	"os"
	"path/filepath"
)

const AppArmorProfileName = ""
const Runtime = "io.containerd.runhcs.v1"

func DataRoot() string {
	return filepath.Join(os.Getenv("ProgramData"), "nerdctl")
}

func CNIPath() string {
	return filepath.Join(os.Getenv("ProgramFiles"), "containerd", "cni", "bin")
}

func CNINetConfPath() string {
	return filepath.Join(os.Getenv("ProgramFiles"), "containerd", "cni", "conf")
}

func NerdctlConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	return filepath.Join(homeDir, "", "nerdctl.toml")
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

func CgroupnsMode() string {
	return ""
}
