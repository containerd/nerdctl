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

package login

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/term"

	"github.com/containerd/log"
)

func readPassword() (string, error) {
	fd := syscall.Stdin
	if !term.IsTerminal(fd) {
		tty, err := os.Open("/dev/tty")
		if err != nil {
			return "", errors.Join(ErrCannotAllocateTerminal, err)
		}
		defer func() {
			err = tty.Close()
			if err != nil {
				log.L.WithError(err).Error("failed closing tty")
			}
		}()
		fd = int(tty.Fd())
	}

	bytePassword, err := term.ReadPassword(fd)
	if err != nil {
		return "", errors.Join(ErrReadingPassword, err)
	}

	return string(bytePassword), nil
}
