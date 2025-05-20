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

// Portions from internal go
//
// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// https://cs.opensource.google/go/go/+/refs/tags/go1.24.3:LICENSE

// https://cs.opensource.google/go/go/+/refs/tags/go1.24.3:src/cmd/go/internal/lockedfile/internal/filelock/filelock_unix.go

package filesystem

import (
	"errors"
	"os"
	"syscall"
)

type lockType int16

const (
	readLock  lockType = syscall.LOCK_SH
	writeLock lockType = syscall.LOCK_EX
)

func platformSpecificLock(file *os.File, lockType lockType) error {
	var err error

	for {
		err = syscall.Flock(int(file.Fd()), int(lockType))
		if !errors.Is(err, syscall.EINTR) {
			break
		}
	}

	return err
}

func platformSpecificUnlock(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
