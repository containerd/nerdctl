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

// https://cs.opensource.google/go/go/+/refs/tags/go1.24.3:src/cmd/go/internal/lockedfile/internal/filelock/filelock_windows.go

package filesystem

import (
	"os"

	"golang.org/x/sys/windows"
)

type lockType uint32

const (
	// https://msdn.microsoft.com/en-us/library/windows/desktop/aa365203(v=vs.85).aspx
	readLock  lockType = 0
	writeLock lockType = windows.LOCKFILE_EXCLUSIVE_LOCK

	reserved = 0
	allBytes = ^uint32(0)
)

func platformSpecificLock(file *os.File, lockType lockType) error {
	return windows.LockFileEx(
		windows.Handle(file.Fd()),
		uint32(lockType),
		reserved,
		allBytes,
		allBytes,
		new(windows.Overlapped))
}

func platformSpecificUnlock(file *os.File) error {
	return windows.UnlockFileEx(windows.Handle(file.Fd()), reserved, allBytes, allBytes, new(windows.Overlapped))
}
