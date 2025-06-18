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

package pkg

import "github.com/containerd/nerdctl/v2/pkg/internal/filesystem"

// InitFS will set the root location to store `internal/filesystem` ops files.
// These files are used to allow `WriteFile` to backup and rollback content.
// While they are transient in nature, they should still persist OS crashes / reboots, so, preferably under something
// like XDGData, rather than tmp.
func InitFS(path string) error {
	return filesystem.SetFilesystemOpsDirectory(path)
}
