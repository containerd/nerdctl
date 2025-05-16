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
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func TestFileStoreBasics(t *testing.T) {
	tempDir := t.TempDir()

	// Creation
	tempStore, err := New(tempDir, 0, 0)
	assert.NilError(t, err, "temporary store creation should succeed")

	// Lock acquisition
	err = tempStore.Lock()
	assert.NilError(t, err, "acquiring a lock should succeed")
	err = tempStore.Release()
	assert.NilError(t, err, "releasing a lock should succeed")

	// Non-existent keys
	_ = tempStore.Lock()
	defer tempStore.Release()

	_, err = tempStore.Get("nonexistent")
	assert.ErrorIs(t, err, ErrNotFound, "getting a non existent key should ErrNotFound")

	err = tempStore.Delete("nonexistent")
	assert.ErrorIs(t, err, ErrNotFound, "deleting a non existent key should ErrNotFound")

	_, err = tempStore.List("nonexistent")
	assert.ErrorIs(t, err, ErrNotFound, "listing a non existent key should ErrNotFound")

	doesExist, err := tempStore.Exists("nonexistent")
	assert.NilError(t, err, "exist should not error")
	assert.Assert(t, !doesExist, "should not exist")

	// Listing empty store
	result, err := tempStore.List()
	assert.NilError(t, err, "listing store root should succeed")
	assert.Assert(t, len(result) == 0, "list empty store return zero length slice")

	// Invalid keys
	_, err = tempStore.Get("..")
	assert.ErrorIs(t, err, ErrInvalidArgument, "unsupported characters or patterns should return ErrInvalidArgument")

	err = tempStore.Set([]byte("foo"), "..")
	assert.ErrorIs(t, err, ErrInvalidArgument, "unsupported characters or patterns should return ErrInvalidArgument")

	err = tempStore.Delete("..")
	assert.ErrorIs(t, err, ErrInvalidArgument, "unsupported characters or patterns should return ErrInvalidArgument")

	_, err = tempStore.List("..")
	assert.ErrorIs(t, err, ErrInvalidArgument, "unsupported characters or patterns should return ErrInvalidArgument")

	// Writing, reading, listing, deleting
	err = tempStore.Set([]byte("foo"), "something")
	assert.NilError(t, err, "write should be successful")

	doesExist, err = tempStore.Exists("something")
	assert.NilError(t, err, "exist should not error")
	assert.Assert(t, doesExist, "should exist")

	data, err := tempStore.Get("something")
	assert.NilError(t, err, "read should be successful")
	assert.Assert(t, string(data) == "foo", "written data should be read back")

	result, err = tempStore.List()
	assert.NilError(t, err, "listing store root should succeed")
	assert.Assert(t, len(result) == 1, "list store with one element should return it")

	// Read from the list key obtained
	data, err = tempStore.Get(result[0])
	assert.NilError(t, err, "read should be successful")
	assert.Assert(t, string(data) == "foo", "written data should be read back")

	err = tempStore.Delete("something")
	assert.NilError(t, err, "delete should be successful")

	doesExist, err = tempStore.Exists("something")
	assert.NilError(t, err, "exist should not error")
	assert.Assert(t, !doesExist, "should not exist")

	result, err = tempStore.List()
	assert.NilError(t, err, "listing store root should succeed")
	assert.Assert(t, len(result) == 0, "list store should return it empty slice")
}

func TestFileStoreGroups(t *testing.T) {
	tempDir := t.TempDir()

	// Creation
	tempStore, err := New(tempDir, 0, 0)
	assert.NilError(t, err, "temporary store creation should succeed")

	_ = tempStore.Lock()
	defer tempStore.Release()

	err = tempStore.Set([]byte("foo"), "group", "subgroup", "actualkey")
	assert.NilError(t, err, "write should be successful")

	doesExist, err := tempStore.Exists("group", "subgroup", "actualkey")
	assert.NilError(t, err, "exist should not error")
	assert.Assert(t, doesExist, "should exist")

	data, err := tempStore.Get("group", "subgroup", "actualkey")
	assert.NilError(t, err, "read should be successful")
	assert.Assert(t, string(data) == "foo", "written data should be read back")

	result, err := tempStore.List()
	assert.NilError(t, err, "listing store root should succeed")
	assert.Assert(t, len(result) == 1)
	assert.Assert(t, result[0] == "group")

	result, err = tempStore.List("group")
	assert.NilError(t, err, "listing store root should succeed")
	assert.Assert(t, len(result) == 1)
	assert.Assert(t, result[0] == "subgroup")

	result, err = tempStore.List("group", "subgroup")
	assert.NilError(t, err, "listing store root should succeed")
	assert.Assert(t, len(result) == 1)
	assert.Assert(t, result[0] == "actualkey")

	err = tempStore.Delete("group", "subgroup", "actualkey")
	assert.NilError(t, err, "delete should be successful")

	doesExist, err = tempStore.Exists("group", "subgroup", "actualkey")
	assert.NilError(t, err, "exist should not error")
	assert.Assert(t, !doesExist, "should not exist")

	doesExist, err = tempStore.Exists("group", "subgroup")
	assert.NilError(t, err, "exist should not error")
	assert.Assert(t, doesExist, "should exist")

	err = tempStore.Delete("group", "subgroup")
	assert.NilError(t, err, "delete should be successful")

	doesExist, err = tempStore.Exists("group", "subgroup")
	assert.NilError(t, err, "exist should not error")
	assert.Assert(t, !doesExist, "should not exist")

}

func TestFileStoreConcurrent(t *testing.T) {
	tempDir := t.TempDir()

	// Creation
	tempStore, err := New(tempDir, 0, 0)
	assert.NilError(t, err, "temporary store creation should succeed")

	go func() {
		lErr := tempStore.WithLock(func() error {
			err := tempStore.Set([]byte("routine 1"), "concurrentkey")
			assert.NilError(t, err, "writing should not error")
			time.Sleep(1 * time.Second)
			result, err := tempStore.Get("concurrentkey")
			assert.NilError(t, err, "reading should not error")
			assert.Assert(t, string(result) == "routine 1")
			return nil
		})
		assert.NilError(t, lErr, "locking should not error")
	}()

	go func() {
		time.Sleep(500 * time.Millisecond)
		lErr := tempStore.WithLock(func() error {
			err := tempStore.Set([]byte("routine 2"), "concurrentkey")
			assert.NilError(t, err, "writing should not error")
			time.Sleep(1 * time.Second)
			result, err := tempStore.Get("concurrentkey")
			assert.NilError(t, err, "reading should not error")
			assert.Assert(t, string(result) == "routine 2")
			return nil
		})
		assert.NilError(t, lErr, "locking should not error")
	}()

	lErr := tempStore.WithLock(func() error {
		err := tempStore.Set([]byte("main routine 1"), "concurrentkey")
		assert.NilError(t, err, "writing should not error")
		time.Sleep(1 * time.Second)
		result, err := tempStore.Get("concurrentkey")
		assert.NilError(t, err, "reading should not error")
		assert.Assert(t, string(result) == "main routine 1")
		return nil
	})
	assert.NilError(t, lErr, "locking should not error")

	time.Sleep(750 * time.Millisecond)

	lErr = tempStore.WithLock(func() error {
		err := tempStore.Set([]byte("main routine 2"), "concurrentkey")
		assert.NilError(t, err, "writing should not error")
		time.Sleep(1 * time.Second)
		result, err := tempStore.Get("concurrentkey")
		assert.NilError(t, err, "reading should not error")
		assert.Assert(t, string(result) == "main routine 2")
		return nil
	})
	assert.NilError(t, lErr, "locking should not error")
}
