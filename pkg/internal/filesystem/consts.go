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

package filesystem

import (
	"io"
	"os"
	"path/filepath"
)

const (
	// Max size of path components
	pathComponentMaxLength = 255
	privateFilePermission  = os.FileMode(0o600)
	privateDirPermission   = os.FileMode(0o700)
)

var (
	// Lightweight indirection to ease testing
	ioCopy = io.Copy

	// Location (under XDG data home) used for markers and backups
	filesystemOpsPath = "filesystem-ops"
	// Suffix for markers and backup files
	markerSuffix = "in-progress"
	backupSuffix = "backup"

	// holdLocation points to where markers and backup files will be held. This should NOT be let to /tmp,
	// but instead be explicitly configured with SetFilesystemOpsDirectory.
	holdLocation = os.TempDir()
)

func SetFilesystemOpsDirectory(path string) error {
	holdLocation = filepath.Join(path, filesystemOpsPath)
	return os.MkdirAll(holdLocation, privateDirPermission)
}
