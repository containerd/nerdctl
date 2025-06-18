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
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	removeMarker = "remove"
)

func ensureRecovery(filename string) (err error) {
	// Check for a marker file.
	// No marker means all fine, nothing to be done.
	// Any other error is a hard error.
	var op string
	if op, err = markerRead(filename); err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		return err
	}

	// We have a marker. We know we were interrupted.
	// Check for a possible backup file.
	var exists bool
	if exists, err = backupExists(filename); err != nil {
		return err
	}

	// If we have a backup, restore from it
	if exists {
		if err = backupRestore(filename); err != nil {
			return err
		}
	} else {
		// We do not see a backup.
		// Do we have a final destination then?
		_, err = os.Stat(filename)
		// Any error but does not exist is a hard error.
		if err != nil && !os.IsNotExist(err) {
			return err
		}

		// If we do NOT have a destination, nothing to be done - we already took care of it, though we were interrupted
		// mid-recovery.

		// If we DO have a destination:
		if err == nil {
			// Either:
			// - there was no original, so we need to remove it (marker contains `remove`)
			// - or we were interrupted ALSO during the recovery attempt, after the backup restore above and before deleting the marker
			// in which case we do NOT want to remove as the file has already been restored.
			if op == removeMarker {
				// Errors on remove are hard errors.
				if err = os.Remove(filename); err != nil {
					return err
				}
			}
		}
	}

	// Ok, we successfully recovered, now, remove the marker and return
	return markerRemove(filename)
}

// backupSave does perform a backup of the provided file at `path`.
func backupSave(path string) error {
	return internalCopy(path, backupLocation(path))
}

// backupRestore restores a file from its backup.
// On success the backup is deleted.
func backupRestore(path string) error {
	err := internalCopy(backupLocation(path), path)
	if err == nil {
		err = os.Remove(backupLocation(path))
	}

	return err
}

// backupExists checks if a backup file exists for file located at `path`.
func backupExists(path string) (bool, error) {
	_, err := os.Stat(backupLocation(path))
	if os.IsNotExist(err) {
		return false, nil
	}

	return err == nil, err
}

// backupLocation returns the location of the backup for path.
func backupLocation(path string) string {
	return location(path) + backupSuffix
}

// markerCreate saves a marker file with the current time.
// Markers are used to indicate an operation is in progress and allow for disaster recovery.
func markerCreate(path string, op string) (err error) {
	var marker *os.File
	marker, err = os.OpenFile(markerLocation(path), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, privateFilePermission)
	if err != nil {
		return err
	}

	defer func() {
		// If we errored on sync or close, remove the marker (ignore removal errors)
		if err = errors.Join(err, marker.Close()); err != nil {
			_ = markerRemove(path)
		}
	}()

	_, err = marker.Write([]byte(op))
	if err != nil {
		return err
	}

	return marker.Sync()
}

// markerRead reads the content of a marker file if it exists (contains the time at which it was created).
func markerRead(path string) (string, error) {
	data, err := os.ReadFile(markerLocation(path))
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// markerRemove deletes a marker file.
func markerRemove(path string) error {
	return os.Remove(markerLocation(path))
}

// markerLocation returns the location of the marker file for a given path.
func markerLocation(path string) string {
	return location(path) + markerSuffix
}

// location returns the filesystem-ops path associated with a given file (where marker and backups are located).
// The location is unique (see hash), and shows the first 16 characters of the filename for readability.
func location(path string) string {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	pretty := base
	// Ensure that we do not blow up filesystem length limits
	if len(pretty) > 16 {
		pretty = pretty[:16]
	}
	return filepath.Join(holdLocation, hash(dir)+"-"+pretty+"-"+hash(base)+"-")
}

// hash does return the first 8 characters of the shasum256 of the provided string.
// Chances of collision are 50% with 77,000 *simultaneous* entries.
func hash(s string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(s)))[0:8]
}

// internalCopy performs a simple copy from source to destination.
// This in itself is not safe.
func internalCopy(sourcePath, destinationPath string) (err error) {
	var source *os.File

	// Open source
	source, err = os.OpenFile(sourcePath, os.O_RDONLY, privateFilePermission)
	if err != nil {
		return err
	}

	// Read file length
	srcInfo, err := source.Stat()
	if err != nil {
		return err
	}

	defer func() {
		err = errors.Join(err, source.Close())
	}()

	return fileWrite(source, srcInfo.Size(), destinationPath, privateFilePermission, srcInfo.ModTime())
}

// fileWrite performs a simple write to the destination file from the provided io.Reader.
// This in itself is not safe.
func fileWrite(source io.Reader, size int64, destinationPath string, perm os.FileMode, mTime time.Time) (err error) {
	var destination *os.File
	mustClose := true

	// Open destination
	destination, err = os.OpenFile(destinationPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}

	defer func() {
		// Close if need be.
		if mustClose {
			err = errors.Join(err, destination.Close())
		}

		// Remove destination if we failed anywhere. Ignore removal failures.
		if err != nil {
			_ = os.Remove(destinationPath)
		}
	}()

	// Copy over
	var n int64
	n, err = ioCopy(destination, source)
	if err != nil {
		return err
	}

	if n < size {
		return io.ErrShortWrite
	}

	// Ensure data is committed
	if err = destination.Sync(); err != nil {
		return err
	}

	err = destination.Close()
	mustClose = false
	if err != nil {
		return err
	}

	if !mTime.IsZero() {
		err = os.Chtimes(destinationPath, mTime, mTime)
	}

	return err
}
