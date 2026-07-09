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

import (
	"fmt"
	"os"

	"github.com/docker/go-units"
	"golang.org/x/sys/unix"
)

// makeShareableDevshm returns devshm directory path on host when there is no error.
func makeShareableDevshm(shmPath, shmSize string) error {
	shmproperty := "mode=1777"
	if len(shmSize) > 0 {
		shmBytes, err := units.RAMInBytes(shmSize)
		if err != nil {
			return err
		}
		shmproperty = fmt.Sprintf("%s,size=%d", shmproperty, shmBytes)
	}
	err := os.MkdirAll(shmPath, 0700)
	if err != nil {
		return err
	}
	err = unix.Mount("/dev/shm", shmPath, "tmpfs", uintptr(unix.MS_NOEXEC|unix.MS_NOSUID|unix.MS_NODEV), shmproperty)
	if err != nil {
		return err
	}

	return nil
}

// cleanUpPlatformSpecificIPC cleans up platform specific IPC.
func cleanUpPlatformSpecificIPC(ipc IPC) error {
	if ipc.Mode == Shareable && ipc.HostShmPath != nil {
		err := unix.Unmount(*ipc.HostShmPath, 0)
		if err != nil {
			return err
		}
		err = os.RemoveAll(*ipc.HostShmPath)
		if err != nil {
			return err
		}
	}
	return nil
}
