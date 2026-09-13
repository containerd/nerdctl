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

package cioutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// CreateStdinFIFO creates a container's stdin FIFO with mode 0600, replacing a
// FIFO left behind by a previous run of the same container.
func CreateStdinFIFO(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := syscall.Mkfifo(path, 0600); err != nil {
		return fmt.Errorf("failed to create the stdin FIFO %q: %w", path, err)
	}
	// Mkfifo is subject to the process umask, so the mode has to be set again.
	return os.Chmod(path, 0600)
}
