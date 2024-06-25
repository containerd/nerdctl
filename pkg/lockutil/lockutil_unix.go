//go:build unix

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

	"golang.org/x/sys/unix"

	"github.com/containerd/log"
)

func WithDirLock(dir string, fn func() error) error {
	dirFile, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer dirFile.Close()
	if err := flock(dirFile, unix.LOCK_EX); err != nil {
		return fmt.Errorf("failed to lock %q: %w", dir, err)
	}
	defer func() {
		if err := flock(dirFile, unix.LOCK_UN); err != nil {
			log.L.WithError(err).Errorf("failed to unlock %q", dir)
		}
	}()
	return fn()
}

func flock(f *os.File, flags int) error {
	fd := int(f.Fd())
	for {
		err := unix.Flock(fd, flags)
		if err == nil || err != unix.EINTR {
			return err
		}
	}
}

func Lock(dir string) (*os.File, error) {
	dirFile, err := os.Open(dir)
	if err != nil {
		return nil, err
	}

	if err = flock(dirFile, unix.LOCK_EX); err != nil {
		return nil, err
	}

	return dirFile, nil
}

func Unlock(locked *os.File) error {
	defer func() {
		_ = locked.Close()
	}()

	if err := flock(locked, unix.LOCK_UN); err != nil {
		return err
	}
	return nil
}
