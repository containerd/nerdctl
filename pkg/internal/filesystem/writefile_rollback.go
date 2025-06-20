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

// WriteFileWithRollback implements an atomic and durable file write operation with rollback.
// The rollback callback may be called by higher-level operations in case there is a need to
// revert changes as part of a more complex, multi-prong operation.
// Note that with or without rollback, WriteFileWithRollback does ensure disaster recovery.
func WriteFileWithRollback(filename string, data []byte, perm os.FileMode) (rollback func() error, err error) {
	// Ensure there are no interrupted operations (leftover marker file and backup), or restore them if need be.
	// If this is failing, we are dead in the water.
	if err = ensureRecovery(filename); err != nil {
		return nil, errors.Join(ErrFilesystemFailure, err)
	}

	// On error, call recovery to rollback changes.
	defer func() {
		if err != nil {
			err = errors.Join(ErrFilesystemFailure, err, ensureRecovery(filename))
		}
	}()

	// If the file does not exist
	markerData := ""
	if _, err = os.Stat(filename); err != nil {
		// Any error but does not exist is a hard error.
		if !os.IsNotExist(err) {
			return nil, err
		}
		// Otherwise, rollback and marker is "remove"
		markerData = removeMarker
		rollback = func() error {
			return os.Remove(filename)
		}
	} else {
		// Destination exists.
		// Rollback will be: restore data from the backup
		rollback = func() error {
			return backupRestore(filename)
		}
	}

	// Create the marker. Failure to do so is a hard error.
	if err = markerCreate(filename, markerData); err != nil {
		return nil, err
	}

	// If the file exists, we need to back it up.
	if markerData == "" {
		// Back it up now.
		if err = backupSave(filename); err != nil {
			return nil, err
		}
	}

	// Now, write the content to the destination.
	if err = fileWrite(bytes.NewReader(data), int64(len(data)), filename, perm, time.Time{}); err != nil {
		return nil, err
	}

	// Remove the marker.
	if err = markerRemove(filename); err != nil {
		return nil, err
	}

	// On success, return the rollback
	return rollback, nil
}
