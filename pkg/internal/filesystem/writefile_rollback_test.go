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

func TestRollbackDoesNotExist(t *testing.T) {
	// Create file
	dir := t.TempDir()
	fp := filepath.Join(dir, "foo")

	// Write to it and check that this went through
	rollback, err := WriteFileWithRollback(fp, []byte("update"), 0o600)
	assert.NilError(t, err)
	cn, _ := os.ReadFile(fp)
	assert.Equal(t, string(cn), "update")

	// Roll it back and check we have the original
	err = rollback()
	assert.NilError(t, err)
	_, err = os.ReadFile(fp)
	assert.Assert(t, os.IsNotExist(err))
}

func TestRollbackExist(t *testing.T) {
	// Create file
	dir := t.TempDir()
	fp := filepath.Join(dir, "foo")
	_ = os.WriteFile(fp, []byte("foo"), 0o600)
	cn, _ := os.ReadFile(fp)
	assert.Equal(t, string(cn), "foo")

	// Write to it and check that this went through
	rollback, err := WriteFileWithRollback(fp, []byte("update"), 0o600)
	assert.NilError(t, err)
	cn, _ = os.ReadFile(fp)
	assert.Equal(t, string(cn), "update")

	// Roll it back and check we have the original
	err = rollback()
	assert.NilError(t, err)
	cn, _ = os.ReadFile(fp)
	assert.Equal(t, string(cn), "foo")
}

func TestInterruptedBackupSave(t *testing.T) {
	// Create file
	dir := t.TempDir()
	fp := filepath.Join(dir, "foo")
	_ = os.WriteFile(fp, []byte("foo"), 0o600)
	cn, _ := os.ReadFile(fp)
	assert.Equal(t, string(cn), "foo")

	fakeError := errors.New("fake error")
	// Override ioCopy to simulate an error creating the backup
	ioCopy = func(dst io.Writer, src io.Reader) (written int64, err error) {
		return 0, fakeError
	}

	// Write. Check that we still have the original.
	rollback, err := WriteFileWithRollback(fp, []byte("update"), 0o600)
	assert.ErrorIs(t, err, fakeError)
	assert.Assert(t, rollback == nil)
	cn, _ = os.ReadFile(fp)
	assert.Equal(t, string(cn), "foo")
}

func TestInterruptedWrite(t *testing.T) {
	// Create file
	dir := t.TempDir()
	fp := filepath.Join(dir, "foo")

	fakeError := errors.New("fake error")
	// Override ioCopy to simulate an error creating the backup
	ioCopy = func(dst io.Writer, src io.Reader) (written int64, err error) {
		return 0, fakeError
	}

	// Write. Check that we still have the original.
	rollback, err := WriteFileWithRollback(fp, []byte("update"), 0o600)
	assert.ErrorIs(t, err, fakeError)
	assert.Assert(t, rollback == nil)
	_, err = os.ReadFile(fp)
	assert.Assert(t, os.IsNotExist(err))

	// Restore io copy
	ioCopy = io.Copy
}

func TestShortWrite(t *testing.T) {
	// Create file
	dir := t.TempDir()
	fp := filepath.Join(dir, "foo")

	// Override ioCopy to simulate a short write
	ioCopy = func(dst io.Writer, src io.Reader) (written int64, err error) {
		return 1, nil
	}

	// Write. Check that we still have the original.
	rollback, err := WriteFileWithRollback(fp, []byte("update"), 0o600)
	assert.ErrorIs(t, err, nil)
	assert.Assert(t, rollback == nil)
	_, err = os.ReadFile(fp)
	assert.Assert(t, os.IsNotExist(err))

	// Restore io copy
	ioCopy = io.Copy
}

func TestDisasterRecoveryFromBackup(t *testing.T) {
	// Create file
	dir := t.TempDir()
	fp := filepath.Join(dir, "foo")
	_ = os.WriteFile(fp, []byte("foo"), 0o600)

	// Artificially create leftover marker
	_ = markerCreate(fp)
	// Artificially create leftover backup
	_ = backupSave(fp)

	// Pork the file
	_ = os.WriteFile(fp, []byte("porked"), 0o600)

	// Now, see that disaster recovery get the backup
	err := EnsureRecovery(fp)
	assert.NilError(t, err)

	cn, _ := os.ReadFile(fp)
	assert.Equal(t, string(cn), "foo")
}

func TestDisasterRecoveryNoBackup1(t *testing.T) {
	// Create file
	dir := t.TempDir()
	fp := filepath.Join(dir, "foo")

	// Artificially create leftover marker
	_ = markerCreate(fp)

	// Pork the file. mtime will be > marker mtime, meaning we expect the file to get deleted
	_ = os.WriteFile(fp, []byte("porked"), 0o600)

	err := EnsureRecovery(fp)
	assert.NilError(t, err)

	_, err = os.ReadFile(fp)
	assert.Assert(t, os.IsNotExist(err))
}

func TestDisasterRecoveryNoBackup2(t *testing.T) {
	// Create file
	dir := t.TempDir()
	fp := filepath.Join(dir, "foo")
	_ = os.WriteFile(fp, []byte("foo"), 0o600)

	// Artificially create leftover marker
	markerCreate(fp)

	err := EnsureRecovery(fp)
	assert.NilError(t, err)

	cn, _ := os.ReadFile(fp)
	assert.Equal(t, string(cn), "foo")
}
