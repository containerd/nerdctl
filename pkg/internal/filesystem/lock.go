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

// https://cs.opensource.google/go/go/+/refs/tags/go1.24.3:src/cmd/go/internal/lockedfile/internal/filelock/filelock.go

package filesystem

import (
	"errors"
	"os"
	"runtime"
)

// Lock places an advisory write lock on the file, blocking until it can be locked.
//
// If Lock returns nil, no other process will be able to place a read or write lock on the file until
// this process exits, closes f, or calls Unlock on it.
func Lock(path string) (file *os.File, err error) {
	return commonlock(path, writeLock)
}

// ReadOnlyLock places an advisory read lock on the file, blocking until it can be locked.
//
// If ReadOnlyLock returns nil, no other process will be able to place a write lock on
// the file until this process exits, closes f, or calls Unlock on it.
func ReadOnlyLock(path string) (file *os.File, err error) {
	return commonlock(path, readLock)
}

func commonlock(path string, mode lockType) (file *os.File, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(ErrLockFail, err, file.Close())
		}
	}()

	if runtime.GOOS == "windows" {
		// LockFileEx does not work on directories, so check what we have first.
		// If that is a dir, swap out the path for a sidecar file instead (not inside the directory).
		// Note that this cannot be done in platform specific implementation without moving all the fd Open and Close
		// logic over there, which is undesirable.
		if sl, err := os.Stat(path); err == nil && sl.IsDir() {
			path = path + ".nerdctl.lock"
		}
	}

	file, err = os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		file, err = os.OpenFile(path, os.O_RDONLY|os.O_CREATE, lockPermission)
	}

	if err != nil {
		return nil, err
	}

	if err = platformSpecificLock(file, mode); err != nil {
		return nil, errors.Join(err, file.Close())
	}

	return file, nil
}

// Unlock removes an advisory lock placed on f by this process.
func Unlock(lock *os.File) error {
	if lock == nil {
		return ErrLockIsNil
	}

	if err := errors.Join(platformSpecificUnlock(lock), lock.Close()); err != nil {
		return errors.Join(ErrUnlockFail, err)
	}

	return nil
}

// WithLock executes the provided function after placing a write lock on `path`.
// The lock is released once the function has been run, regardless of outcome.
func WithLock(path string, function func() error) (err error) {
	file, err := Lock(path)
	if err != nil {
		return err
	}

	defer func() {
		err = errors.Join(Unlock(file), err)
	}()

	return function()
}

// WithReadOnlyLock executes the provided function after placing a read lock on `path`.
// The lock is released once the function has been run, regardless of outcome.
func WithReadOnlyLock(path string, function func() error) (err error) {
	file, err := ReadOnlyLock(path)
	if err != nil {
		return err
	}

	defer func() {
		err = errors.Join(Unlock(file), err)
	}()

	return function()
}
