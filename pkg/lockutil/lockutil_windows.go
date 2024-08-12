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

package lockutil

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"

	"github.com/containerd/log"
)

func WithDirLock(dir string, fn func() error) error {
	dirFile, err := os.OpenFile(dir+".lock", os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer dirFile.Close()
	// see https://msdn.microsoft.com/en-us/library/windows/desktop/aa365203(v=vs.85).aspx
	if err = windows.LockFileEx(windows.Handle(dirFile.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, ^uint32(0), ^uint32(0), new(windows.Overlapped)); err != nil {
		return fmt.Errorf("failed to lock %q: %w", dir, err)
	}

	defer func() {
		if err := windows.UnlockFileEx(windows.Handle(dirFile.Fd()), 0, ^uint32(0), ^uint32(0), new(windows.Overlapped)); err != nil {
			log.L.WithError(err).Errorf("failed to unlock %q", dir)
		}
	}()
	return fn()
}

func Lock(dir string) (*os.File, error) {
	dirFile, err := os.OpenFile(dir+".lock", os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	// see https://msdn.microsoft.com/en-us/library/windows/desktop/aa365203(v=vs.85).aspx
	if err = windows.LockFileEx(windows.Handle(dirFile.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, ^uint32(0), ^uint32(0), new(windows.Overlapped)); err != nil {
		return nil, fmt.Errorf("failed to lock %q: %w", dir, err)
	}
	return dirFile, nil
}

func Unlock(locked *os.File) error {
	defer func() {
		_ = locked.Close()
	}()

	return windows.UnlockFileEx(windows.Handle(locked.Fd()), 0, ^uint32(0), ^uint32(0), new(windows.Overlapped))
}
