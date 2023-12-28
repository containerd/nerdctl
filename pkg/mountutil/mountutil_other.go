//go:build !windows

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

package mountutil

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/containerd/nerdctl/pkg/mountutil/volumestore"
)

func splitVolumeSpec(s string) ([]string, error) {
	s = strings.TrimLeft(s, ":")
	split := strings.Split(s, ":")
	return split, nil
}

func handleVolumeToMount(source string, dst string, volStore volumestore.VolumeStore, createDir bool) (volumeSpec, error) {
	switch {
	// Handle named volumes
	case isNamedVolume(source):
		return handleNamedVolumes(source, volStore)

	// Handle bind volumes (file paths)
	default:
		return handleBindMounts(source, createDir)
	}
}

func cleanMount(p string) string {
	return filepath.Clean(p)
}

func isValidPath(s string) (bool, error) {
	if filepath.IsAbs(s) {
		return true, nil
	}

	return false, fmt.Errorf("expected an absolute path, got %q", s)
}

/*
For docker compatibility on non-Windows platforms:
Docker allows anonymous named volumes, relative paths, and absolute paths
to be mounted into a container.
*/
func validateAnonymousVolumeDestination(s string) (bool, error) {
	return true, nil
}
