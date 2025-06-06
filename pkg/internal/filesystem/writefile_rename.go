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

package filesystem

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

// WriteFileWithRename is a drop-in replacement for os.WriteFile, with the same signature and almost identical behavior
// (see note below on inodes).
// Unlike os.WriteFile, it does provide extra guarantees:
// - Atomicity (provided by rename - *mostly* atomic, except on OS crash, where rename behavior is undefined)
// - Durability (sync-ed)
// Note that:
// - this does not provide Isolation (a locking mechanism needs to be used independently to enforce that)
// - Consistency is orthogonal here, and high-level operations that expect it across a set of unrelated ops need to
// implement locking, rollback, and disaster recovery
// - this will change inode in case the file already exist - therefore, there are cases where this cannot be used
// (specifically if a file is mounted inside a container) - these are the exception though, and in almost all cases,
// this method should be preferred over os.WriteFile
// Finally note that we do not do anything smart wrt symlinks.
// User is expected to resolve symlink for the destination before calling this if needed.
func WriteFileWithRename(filename string, data []byte, perm os.FileMode) error {
	return CopyToFileWithRename(filename, bytes.NewBuffer(data), int64(len(data)), perm, time.Time{})
}

// CopyToFileWithRename is an atomic wrapper around io.Copy(file, reader). See notes above in WriteFile for details.
func CopyToFileWithRename(filename string, reader io.Reader, dataSize int64, perm os.FileMode, mTime time.Time) (err error) {
	var tmpFile *os.File
	mustClose := true

	defer func() {
		// Close if we have not already
		if mustClose {
			err = errors.Join(err, tmpFile.Close())
		}

		// On error, wrap it into ErrFilesystemFailure (and ensure we don't leak temp files)
		if err != nil {
			if tmpFile != nil {
				err = errors.Join(err, os.Remove(tmpFile.Name()))
			}
			err = errors.Join(ErrFilesystemFailure, err)
		}
	}()

	// Ensure we set permission honoring umask to be compatible with os.WriteFile
	perm = (^os.FileMode(GetUmask())) & perm

	// Create a new temp file.
	tmpFile, err = os.CreateTemp(filepath.Dir(filename), ".tmp-"+filepath.Base(filename))
	if err != nil {
		return err
	}

	// Set permissions
	if err = os.Chmod(tmpFile.Name(), perm); err != nil {
		return err
	}

	// Write data
	n, err := ioCopy(tmpFile, reader)
	if err == nil && n < dataSize {
		return io.ErrShortWrite
	}

	if err != nil {
		return err
	}

	// Sync it, ensuring the data cannot be lost
	if err = tmpFile.Sync(); err != nil {
		return err
	}

	// Close
	if err = tmpFile.Close(); err != nil {
		return err
	}

	mustClose = false

	// Set mtime if requested
	if !mTime.IsZero() {
		if err = os.Chtimes(tmpFile.Name(), mTime, mTime); err != nil {
			return err
		}
	}

	// Rename to final destination (hopefully on the same volume)
	// NOTE: this is atomic in *most* cases - it might not be if the OS crashes.
	return os.Rename(tmpFile.Name(), filename)
}
