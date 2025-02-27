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

package pty

import (
	"errors"
	"os"
	"strconv"
	"syscall"
	"unsafe"
)

// Inspiration from https://github.com/creack/pty/tree/2cde18bfb702199728dd43bf10a6c15c7336da0a

func Open() (pty, tty *os.File, err error) {
	// Wrap errors
	defer func() {
		if err != nil && pty != nil {
			err = errors.Join(pty.Close(), err)
		}
		if err != nil {
			err = errors.Join(ErrPTYFailure, err)
		}
	}()

	// Open the pty
	pty, err = os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}

	// Get the slave unit number
	var n uint32
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, pty.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&n)))
	if e != 0 {
		return nil, nil, e
	}

	sname := "/dev/pts/" + strconv.Itoa(int(n))

	// Unlock
	var u int32
	_, _, e = syscall.Syscall(syscall.SYS_IOCTL, pty.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&u)))
	if e != 0 {
		return nil, nil, e
	}

	// Open the slave, preventing it from becoming the controlling terminal
	tty, err = os.OpenFile(sname, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, err
	}

	return pty, tty, nil
}
