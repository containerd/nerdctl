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

// This is a dummy file to allow usage of library functions
// on Darwin-based systems.
// All functions and variables are empty/no-ops

package defaults

func CNIPath() string {
	return ""
}

func CNINetConfPath() string {
	return ""
}

func DataRoot() string {
	return ""
}

func CgroupManager() string {
	return ""
}

func HostsDirs() []string {
	return []string{}
}

func HostGatewayIP() string {
	return ""
}
