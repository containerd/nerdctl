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

package tarutil

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/containerd/log"
)

// FindTarBinary returns a path to the tar binary and whether it is GNU tar.
func FindTarBinary() (string, bool, error) {
	isGNU := func(exe string) bool {
		v, err := exec.Command(exe, "--version").Output()
		if err != nil {
			log.L.Warnf("Failed to detect whether %q is GNU tar or not", exe)
			return false
		}
		if !strings.Contains(string(v), "GNU tar") {
			log.L.Warnf("%q does not seem GNU tar", exe)
			return false
		}
		return true
	}
	if v := os.Getenv("TAR"); v != "" {
		if exe, err := exec.LookPath(v); err == nil {
			return exe, isGNU(exe), nil
		}
	}
	if exe, err := exec.LookPath("gnutar"); err == nil {
		return exe, true, nil
	}
	if exe, err := exec.LookPath("gtar"); err == nil {
		return exe, true, nil
	}
	if exe, err := exec.LookPath("tar"); err == nil {
		return exe, isGNU(exe), nil
	}
	return "", false, fmt.Errorf("failed to find `tar` binary")
}
