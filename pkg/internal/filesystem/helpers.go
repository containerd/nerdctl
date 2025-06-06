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
	// Permission used for markers and backups
	privateFilePerm = 0o600
	privateDirPerm  = 0o700
	// Location (under XDG data home) used for markers and backups
	locationPath = "filesystem-ops"
	// Suffix for markers and backup files
	markerSuffix = "in-progress"
	backupSuffix = "backup"
)

var (
	// Lightweight indirection which only purpose it to facilitate mocking ioCopy for testing.
	ioCopy = io.Copy

	// Location where markers and backup files will be held. This should NOT be let to /tmp, but instead be
	// explicitly configured with SetFSOpsDirectory
	holdLocation = "/tmp"

	// If a marker is a dir, hard error
	errMarkerIsADir = errors.New("marker file is a directory")
)

func SetFSOpsDirectory(path string) error {
	holdLocation = path
	return os.MkdirAll(filepath.Join(holdLocation, locationPath), privateDirPerm)
}

// Note: 50% chance of a collision with 77,000 *simultaneous* entries.
func hash(s string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(s)))[0:8]
}

func location(path string) string {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	pretty := base
	// Ensure that we do not blow up filesystem length limits
	if len(pretty) > 16 {
		pretty = pretty[:16]
	}
	return filepath.Join(holdLocation, locationPath, hash(dir)+"-"+pretty+"-"+hash(base)+"-")
}

func markerLocation(path string) string {
	return location(path) + markerSuffix
}

func backupLocation(path string) string {
	return location(path) + backupSuffix
}

func markerCreate(path string) (err error) {
	// NOTE: this is far from perfect. Right now, we rely on mtimes of the marker and destination
	// to figure out recovery strategy, so, we need them be different.
	time.Sleep(10 * time.Millisecond)
	var marker *os.File

	marker, err = os.OpenFile(markerLocation(path), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, privateFilePerm)
	if err != nil {
		return err
	}

	defer func() {
		// If we errored on sync or close, remove the marker
		if err = errors.Join(err, marker.Close()); err != nil {
			err = errors.Join(err, markerRemove(path))
		}
		time.Sleep(10 * time.Millisecond)
	}()

	return marker.Sync()
}

func markerModTime(path string) (time.Time, error) {
	markerState, err := os.Stat(markerLocation(path))
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		return time.Time{}, err
	}

	if markerState.IsDir() {
		return time.Time{}, errMarkerIsADir
	}

	return markerState.ModTime(), nil
}

func markerRemove(path string) error {
	return os.Remove(markerLocation(path))
}

func backupSave(path string) error {
	return internalCopy(path, backupLocation(path))
}

func backupRestore(path string) error {
	err := internalCopy(backupLocation(path), path)
	if err == nil {
		err = os.Remove(backupLocation(path))
	}

	return err
}

func backupExists(path string) (bool, error) {
	_, err := os.Stat(backupLocation(path))
	if os.IsNotExist(err) {
		return false, nil
	}

	return err == nil, err
}

func internalCopy(sourcePath, destinationPath string) (err error) {
	var source *os.File

	// Open source
	source, err = os.OpenFile(sourcePath, os.O_RDONLY, privateFilePerm)
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

	return fileWrite(source, srcInfo.Size(), destinationPath, privateFilePerm, srcInfo.ModTime())
}

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
