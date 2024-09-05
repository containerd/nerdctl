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

package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/containerd/nerdctl/v2/pkg/lockutil"
)

// TODO: implement a read-lock in lockutil, in addition to the current exclusive write-lock
// This might improve performance in case of (mostly read) massively parallel concurrent scenarios

const (
	// Default filesystem permissions to use when creating dir or files
	defaultFilePerm = 0o600
	defaultDirPerm  = 0o700
)

// New returns a filesystem based Store implementation that satisfies both Manager and Locker
// Note that atomicity is "guaranteed" by `os.Rename`, which arguably is not *always* atomic.
// In particular, operating-system crashes may break that promise, and windows behavior is probably questionable.
// That being said, this is still a much better solution than writing directly to the destination file.
func New(rootPath string, dirPerm os.FileMode, filePerm os.FileMode) (Store, error) {
	if rootPath == "" {
		return nil, errors.Join(ErrInvalidArgument, fmt.Errorf("FileStore rootPath cannot be empty"))
	}

	if dirPerm == 0 {
		dirPerm = defaultDirPerm
	}

	if filePerm == 0 {
		filePerm = defaultFilePerm
	}

	if err := os.MkdirAll(rootPath, dirPerm); err != nil {
		return nil, errors.Join(ErrSystemFailure, err)
	}

	return &fileStore{
		dir:      rootPath,
		dirPerm:  dirPerm,
		filePerm: filePerm,
	}, nil
}

type fileStore struct {
	mutex    sync.RWMutex
	dir      string
	locked   *os.File
	dirPerm  os.FileMode
	filePerm os.FileMode
}

func (vs *fileStore) Lock() error {
	vs.mutex.Lock()

	dirFile, err := lockutil.Lock(vs.dir)
	if err != nil {
		return errors.Join(ErrLockFailure, err)
	}

	vs.locked = dirFile

	return nil
}

func (vs *fileStore) Release() error {
	if vs.locked == nil {
		return errors.Join(ErrFaultyImplementation, fmt.Errorf("cannot unlock already unlocked volume store %q", vs.dir))
	}

	defer vs.mutex.Unlock()

	defer func() {
		vs.locked = nil
	}()

	if err := lockutil.Unlock(vs.locked); err != nil {
		return errors.Join(ErrLockFailure, err)
	}

	return nil
}

func (vs *fileStore) WithLock(fun func() error) (err error) {
	if err = vs.Lock(); err != nil {
		return err
	}

	defer func() {
		err = errors.Join(vs.Release(), err)
	}()

	return fun()
}

func (vs *fileStore) Get(key ...string) ([]byte, error) {
	if vs.locked == nil {
		return nil, errors.Join(ErrFaultyImplementation, fmt.Errorf("operations on the store must use locking"))
	}

	if err := validateAllPathComponents(key...); err != nil {
		return nil, err
	}

	path := filepath.Join(append([]string{vs.dir}, key...)...)

	st, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errors.Join(ErrNotFound, fmt.Errorf("%q does not exist", filepath.Join(key...)))
		}

		return nil, errors.Join(ErrSystemFailure, err)
	}

	if st.IsDir() {
		return nil, errors.Join(ErrFaultyImplementation, fmt.Errorf("%q is a directory and cannot be read as a file", path))
	}

	content, err := os.ReadFile(filepath.Join(append([]string{vs.dir}, key...)...))
	if err != nil {
		return nil, errors.Join(ErrSystemFailure, err)
	}

	return content, nil
}

func (vs *fileStore) Exists(key ...string) (bool, error) {
	if err := validateAllPathComponents(key...); err != nil {
		return false, err
	}

	path := filepath.Join(append([]string{vs.dir}, key...)...)

	_, err := os.Stat(filepath.Join(path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, errors.Join(ErrSystemFailure, err)
	}

	return true, nil
}

func (vs *fileStore) Set(data []byte, key ...string) error {
	if vs.locked == nil {
		return errors.Join(ErrFaultyImplementation, fmt.Errorf("operations on the store must use locking"))
	}

	if err := validateAllPathComponents(key...); err != nil {
		return err
	}

	fileName := key[len(key)-1]
	parent := vs.dir

	if len(key) > 1 {
		parent = filepath.Join(append([]string{parent}, key[0:len(key)-1]...)...)
		err := os.MkdirAll(parent, vs.dirPerm)
		if err != nil {
			return errors.Join(ErrSystemFailure, err)
		}
	}

	dest := filepath.Join(parent, fileName)
	st, err := os.Stat(dest)
	if err == nil {
		if st.IsDir() {
			return errors.Join(ErrFaultyImplementation, fmt.Errorf("%q is a directory and cannot be written to", dest))
		}
	}

	return atomicWrite(parent, fileName, vs.filePerm, data)
}

func (vs *fileStore) List(key ...string) ([]string, error) {
	if vs.locked == nil {
		return nil, errors.Join(ErrFaultyImplementation, fmt.Errorf("operations on the store must use locking"))
	}

	// Unlike Get, Set and Delete, List can have zero length key
	for _, k := range key {
		if err := validatePathComponent(k); err != nil {
			return nil, err
		}
	}

	path := filepath.Join(append([]string{vs.dir}, key...)...)

	st, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errors.Join(ErrNotFound, err)
		}

		return nil, errors.Join(ErrSystemFailure, err)
	}

	if !st.IsDir() {
		return nil, errors.Join(ErrFaultyImplementation, fmt.Errorf("%q is not a directory and cannot be enumerated", path))
	}

	dirEntries, err := os.ReadDir(path)
	if err != nil {
		return nil, errors.Join(ErrSystemFailure, err)
	}

	entries := []string{}
	for _, dirEntry := range dirEntries {
		entries = append(entries, dirEntry.Name())
	}

	return entries, nil
}

func (vs *fileStore) Delete(key ...string) error {
	if vs.locked == nil {
		return errors.Join(ErrFaultyImplementation, fmt.Errorf("operations on the store must use locking"))
	}

	if err := validateAllPathComponents(key...); err != nil {
		return err
	}

	path := filepath.Join(append([]string{vs.dir}, key...)...)

	_, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.Join(ErrNotFound, err)
		}

		return errors.Join(ErrSystemFailure, err)
	}

	if err = os.RemoveAll(path); err != nil {
		return errors.Join(ErrSystemFailure, err)
	}

	return nil
}

func (vs *fileStore) Location(key ...string) (string, error) {
	if err := validateAllPathComponents(key...); err != nil {
		return "", err
	}

	return filepath.Join(append([]string{vs.dir}, key...)...), nil
}

func (vs *fileStore) GroupEnsure(key ...string) error {
	if vs.locked == nil {
		return errors.Join(ErrFaultyImplementation, fmt.Errorf("operations on the store must use locking"))
	}

	if err := validateAllPathComponents(key...); err != nil {
		return err
	}

	path := filepath.Join(append([]string{vs.dir}, key...)...)

	if err := os.MkdirAll(path, vs.dirPerm); err != nil {
		return errors.Join(ErrSystemFailure, err)
	}

	return nil
}

func (vs *fileStore) GroupSize(key ...string) (int64, error) {
	if vs.locked == nil {
		return 0, errors.Join(ErrFaultyImplementation, fmt.Errorf("operations on the store must use locking"))
	}

	if err := validateAllPathComponents(key...); err != nil {
		return 0, err
	}

	path := filepath.Join(append([]string{vs.dir}, key...)...)

	st, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, errors.Join(ErrNotFound, err)
		}

		return 0, errors.Join(ErrSystemFailure, err)
	}

	if !st.IsDir() {
		return 0, errors.Join(ErrFaultyImplementation, fmt.Errorf("%q is not a directory", path))
	}

	var size int64
	var walkFn = func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	}

	err = filepath.Walk(path, walkFn)
	if err != nil {
		return 0, err
	}

	return size, nil
}

// validatePathComponent will enforce os specific filename restrictions on a single path component
func validatePathComponent(pathComponent string) error {
	// https://en.wikipedia.org/wiki/Comparison_of_file_systems#Limits
	if len(pathComponent) > 255 {
		return errors.Join(ErrInvalidArgument, errors.New("identifiers must be stricly shorter than 256 characters"))
	}

	if strings.TrimSpace(pathComponent) == "" {
		return errors.Join(ErrInvalidArgument, errors.New("identifier cannot be empty"))
	}

	if err := validatePlatformSpecific(pathComponent); err != nil {
		return errors.Join(ErrInvalidArgument, err)
	}

	return nil
}

// validateAllPathComponents will enforce validation for a slice of components
func validateAllPathComponents(pathComponent ...string) error {
	if len(pathComponent) == 0 {
		return errors.Join(ErrInvalidArgument, errors.New("you must specify an identifier"))
	}

	for _, key := range pathComponent {
		if err := validatePathComponent(key); err != nil {
			return err
		}
	}

	return nil
}

func atomicWrite(parent string, fileName string, perm os.FileMode, data []byte) error {
	dest := filepath.Join(parent, fileName)
	temp := filepath.Join(parent, ".temp."+fileName)

	err := os.WriteFile(temp, data, perm)
	if err != nil {
		return errors.Join(ErrSystemFailure, err)
	}

	err = os.Rename(temp, dest)
	if err != nil {
		return errors.Join(ErrSystemFailure, err)
	}

	return nil
}
