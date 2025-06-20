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

//nolint:forbidigo
package filesystem

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

func TestRollbackForNonExistentFile(t *testing.T) {
	// Test that calling the rollback after writing to a new existent file does remove the file

	// Create file
	dir := t.TempDir()
	fp := filepath.Join(dir, "non-existent-file")

	// Write to it and check that this went through
	rollback, err := WriteFileWithRollback(fp, []byte("new content"), 0o600)
	assert.NilError(t, err)
	cn, _ := os.ReadFile(fp)
	assert.Equal(t, string(cn), "new content")

	// Roll it back and check it has been removed.
	err = rollback()
	assert.NilError(t, err)
	_, err = os.ReadFile(fp)
	assert.Assert(t, os.IsNotExist(err))
}

func TestRollbackForPreexistingFile(t *testing.T) {
	// Test that calling the rollback after writing to a pre-existing file does restore the original

	// Create a file with pre-existing content
	dir := t.TempDir()
	fp := filepath.Join(dir, "pre-existing-file")
	_ = os.WriteFile(fp, []byte("original content"), 0o600)
	cn, _ := os.ReadFile(fp)
	assert.Equal(t, string(cn), "original content")

	// Write to it and check that this went through
	rollback, err := WriteFileWithRollback(fp, []byte("updated content"), 0o600)
	assert.NilError(t, err)

	cn, _ = os.ReadFile(fp)
	assert.Equal(t, string(cn), "updated content")

	// Roll it back and check we have the original
	err = rollback()
	assert.NilError(t, err)
	cn, _ = os.ReadFile(fp)
	assert.Equal(t, string(cn), "original content")
}

func TestBackupFailure(t *testing.T) {
	// Test that if backup is failing, a pre-existing file is restored to its original value.

	// Create a file with pre-existing content
	dir := t.TempDir()
	fp := filepath.Join(dir, "pre-existing-file")
	_ = os.WriteFile(fp, []byte("original content"), 0o600)
	cn, _ := os.ReadFile(fp)
	assert.Equal(t, string(cn), "original content")

	fakeError := errors.New("fake error")
	// Override ioCopy to simulate an error creating the backup
	ioCopy = func(dst io.Writer, src io.Reader) (written int64, err error) {
		return 0, fakeError
	}

	// Write. Check that we still have the original.
	rollback, err := WriteFileWithRollback(fp, []byte("updated content"), 0o600)
	assert.ErrorIs(t, err, fakeError)
	assert.Assert(t, rollback == nil)
	cn, _ = os.ReadFile(fp)
	assert.Equal(t, string(cn), "original content")
}

func TestWriteFailure(t *testing.T) {
	// Test that if write to a non-existent file is failing, the file is deleted.

	// Create file
	dir := t.TempDir()
	fp := filepath.Join(dir, "non-existent-file")

	fakeError := errors.New("fake error")
	// Override ioCopy to simulate an error while writing to the destination
	// Note: since the file does not exist, there will be no backup
	ioCopy = func(dst io.Writer, src io.Reader) (written int64, err error) {
		return 0, fakeError
	}

	// Write. Check that the file has been removed
	rollback, err := WriteFileWithRollback(fp, []byte("update"), 0o600)
	assert.ErrorIs(t, err, fakeError)
	assert.Assert(t, rollback == nil)
	_, err = os.ReadFile(fp)
	assert.Assert(t, os.IsNotExist(err))

	// Restore io copy
	ioCopy = io.Copy
}

func TestShortWriteFailure(t *testing.T) {
	// Test that a write failing to write all content to a non-existent file will delete the file.

	// Create file
	dir := t.TempDir()
	fp := filepath.Join(dir, "non-existent-file")

	// Override ioCopy to simulate a short write
	ioCopy = func(dst io.Writer, src io.Reader) (written int64, err error) {
		return 1, nil
	}

	// Write. Check that we still have the original.
	rollback, err := WriteFileWithRollback(fp, []byte("update"), 0o600)
	assert.ErrorIs(t, err, io.ErrShortWrite)
	assert.Assert(t, rollback == nil)
	_, err = os.ReadFile(fp)
	assert.Assert(t, os.IsNotExist(err))

	// Restore io copy
	ioCopy = io.Copy
}

func TestDisasterRecoveryFromBackup(t *testing.T) {
	// Test that a file that has left-over backup and marker will get restored to its original content

	// Create file
	dir := t.TempDir()
	fp := filepath.Join(dir, "pre-existing-file")
	_ = os.WriteFile(fp, []byte("original content"), 0o600)

	// Artificially create leftover marker
	_ = markerCreate(fp, "")
	// Artificially create leftover backup
	_ = backupSave(fp)

	// Pork the file, to simulate interrupted write with leftover marker and backup
	_ = os.WriteFile(fp, []byte("porked"), 0o600)

	// Now, see that disaster recovery got the backup
	err := ensureRecovery(fp)
	assert.NilError(t, err)

	cn, _ := os.ReadFile(fp)
	assert.Equal(t, string(cn), "original content")
}

func TestDisasterRecoveryNoBackup1(t *testing.T) {
	// Test that a previously non-existent file with a marker left-over will get deleted

	// Create file
	dir := t.TempDir()
	fp := filepath.Join(dir, "non-existent-file")

	// Artificially create leftover marker
	_ = markerCreate(fp, removeMarker)

	// Pork the file. mtime will be > marker mtime, meaning we expect the file to get deleted
	_ = os.WriteFile(fp, []byte("porked"), 0o600)

	err := ensureRecovery(fp)
	assert.NilError(t, err)

	_, err = os.ReadFile(fp)
	assert.Assert(t, os.IsNotExist(err))
}

func TestDisasterRecoveryNoBackup2(t *testing.T) {
	// Test that a file with a more recent marker leftover and no backup will be left untouched.

	// Create file
	dir := t.TempDir()
	fp := filepath.Join(dir, "pre-existing-file")
	_ = os.WriteFile(fp, []byte("original content"), 0o600)

	// Artificially create leftover marker
	_ = markerCreate(fp, "")

	err := ensureRecovery(fp)
	assert.NilError(t, err)

	cn, _ := os.ReadFile(fp)
	assert.Equal(t, string(cn), "original content")
}
