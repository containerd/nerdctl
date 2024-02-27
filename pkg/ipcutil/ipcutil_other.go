//go:build !(linux || windows)

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

package ipcutil

import "fmt"

// makeShareableDevshm returns devshm directory path on host when there is no error.
func makeShareableDevshm(shmPath, shmSize string) error {
	return fmt.Errorf("unix does not support shareable devshm")
}

// cleanUpPlatformSpecificIPC cleans up platform specific IPC.
func cleanUpPlatformSpecificIPC(ipc IPC) error {
	if ipc.Mode == Shareable {
		return fmt.Errorf("unix does not support shareable devshm")
	}
	return nil
}
