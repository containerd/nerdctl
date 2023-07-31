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

	"github.com/containerd/log"
	"golang.org/x/sys/windows"
)

func WithLock(name string, fn func() error) error {
	dirFile, err := os.OpenFile(name+".lock", os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer dirFile.Close()
	// see https://msdn.microsoft.com/en-us/library/windows/desktop/aa365203(v=vs.85).aspx
	// 1 lock immediately
	if err = windows.LockFileEx(windows.Handle(dirFile.Fd()), 1, 0, 1, 0, &windows.Overlapped{}); err != nil {
		return fmt.Errorf("failed to lock %q: %w", name, err)
	}

	defer func() {
		if err := windows.UnlockFileEx(windows.Handle(dirFile.Fd()), 0, 1, 0, &windows.Overlapped{}); err != nil {
			log.L.WithError(err).Errorf("failed to unlock %q", name)
		}
	}()
	return fn()
}
