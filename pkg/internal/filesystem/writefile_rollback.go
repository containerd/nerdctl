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
	"os"
	"time"
)

// WriteFile implements an atomic and durable alternative to os.WriteFile that does not change inodes (unlike the usual
// approach on atomic writes that relies on renaming files).
func WriteFile(filename string, data []byte, perm os.FileMode) error {
	_, err := WriteFileWithRollback(filename, data, perm)
	return err
}

func WriteFileWithRollback(filename string, data []byte, perm os.FileMode) (rollback func() error, err error) {
	// Start with handling any leftover past disaster by restoring the original to what it initially was if need be.
	// If this is failing, we are dead in the water.
	if err = EnsureRecovery(filename); err != nil {
		return nil, err
	}

	// On error, call disaster.
	defer func() {
		if err != nil {
			err = errors.Join(err, EnsureRecovery(filename))
		}
	}()

	// Create a marker. Failure to do so is hard.
	if err = markerCreate(filename); err != nil {
		return nil, err
	}

	// Check the destination file
	if _, err = os.Stat(filename); err != nil {
		// Any error but does not exist is a hard error.
		if !os.IsNotExist(err) {
			return nil, err
		}

		// Destination does not exist.
		// Rollback is: remove it.
		rollback = func() error {
			return os.Remove(filename)
		}
	} else {
		// Destination exists.
		// Rollback is: restore data from the backup
		rollback = func() error {
			return backupRestore(filename)
		}

		// Back it up now.
		if err = backupSave(filename); err != nil {
			return nil, err
		}
	}

	// Write the content to the destination.
	if err = fileWrite(bytes.NewReader(data), int64(len(data)), filename, perm, time.Time{}); err != nil {
		return nil, err
	}

	// Remove the marker
	if err = markerRemove(filename); err != nil {
		return nil, err
	}

	// On success, return the rollback
	return rollback, nil
}

func EnsureRecovery(filename string) (err error) {
	var destinationState os.FileInfo

	// Check for a marker file. Hard errors are any error but IsNotExist.
	var mmtime time.Time
	mmtime, err = markerModTime(filename)
	if err != nil {
		return err
	}

	// Mod time zero and no error means it does not exist. No recovery needed then.
	if mmtime.IsZero() {
		return nil
	}

	// We have a file marker. We know we were interrupted.
	// Check for a possible backup file.
	var exists bool
	exists, err = backupExists(filename)
	if err != nil {
		return err
	}

	// If we have one, restore from it
	if exists {
		if err = backupRestore(filename); err != nil {
			return err
		}
	} else {
		// We do not see a backup.
		// Do we have a final destination then?
		destinationState, err = os.Stat(filename)
		// Any error but does not exist is a hard error.
		if err != nil && !os.IsNotExist(err) {
			return err
		}

		// If we do not have a destination, nothing to be done - we already took care of it, though we were interrupted
		// mid-recovery.
		// If we DO have a destination:
		if err == nil {
			// Either:
			// - there was no original (destination was non-existent), so we need to remove it - the mtime will be > the marker mtime
			// - or we were interrupted ALSO during the recovery attempt, after the backup restore above and before deleting the marker
			// in which case we do NOT want to remove as the file has already been restored, with a mtime that is < the marker time.
			if mmtime.Before(destinationState.ModTime()) {
				err = os.Remove(filename)
				// Errors on remove are hard errors.
				if err != nil {
					return err
				}
			}
		}
	}

	// Ok, we successfully recovered, now, remove the marker and return
	return markerRemove(filename)
}
