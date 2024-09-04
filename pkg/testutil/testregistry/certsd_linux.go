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

package testregistry

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
)

func generateCertsd(dir string, certPath string, hostIP string, port int) error {
	joined := hostIP
	if port != 0 {
		joined = net.JoinHostPort(hostIP, strconv.Itoa(port))
	}

	hostsSubDir := filepath.Join(dir, joined)
	err := os.MkdirAll(hostsSubDir, 0700)
	if err != nil {
		return err
	}

	hostsTOMLPath := filepath.Join(hostsSubDir, "hosts.toml")
	// See https://github.com/containerd/containerd/blob/main/docs/hosts.md
	hostsTOML := fmt.Sprintf(`
server = "https://%s"
[host."https://%s"]
  ca = %q
		`, joined, joined, certPath)
	return os.WriteFile(hostsTOMLPath, []byte(hostsTOML), 0700)
}
