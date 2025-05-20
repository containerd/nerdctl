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

package filesystem_test

import (
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/internal/filesystem"
)

const (
	mainroutine1 uint32 = 11
	mainroutine2 uint32 = 12
	routine1     uint32 = 1
	routine2     uint32 = 2
	routine3     uint32 = 3
)

func TestLockDir(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	// Lock acquisition
	file, err := filesystem.Lock(tempDir)
	assert.NilError(t, err, "acquiring a lock should succeed")
	err = filesystem.Unlock(file)
	assert.NilError(t, err, "releasing a lock should succeed")

	file, err = filesystem.ReadOnlyLock(tempDir)
	assert.NilError(t, err, "acquiring a read-only lock should succeed")
	file2, err := filesystem.ReadOnlyLock(tempDir)
	assert.NilError(t, err, "acquiring another read-only lock should succeed")
	err = filesystem.Unlock(file)
	assert.NilError(t, err, "releasing a read-only lock should succeed")
	err = filesystem.Unlock(file2)
	assert.NilError(t, err, "releasing another read-only lock should succeed")
}

func TestLockFile(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	lock, err := os.CreateTemp(tempDir, "lockfile")
	assert.NilError(t, err, "creating temp file should succeed")
	defer lock.Close()
	// Lock acquisition
	file, err := filesystem.Lock(lock.Name())
	assert.NilError(t, err, "acquiring a lock should succeed")
	err = filesystem.Unlock(file)
	assert.NilError(t, err, "releasing a lock should succeed")

	file, err = filesystem.ReadOnlyLock(lock.Name())
	assert.NilError(t, err, "acquiring a read-only lock should succeed")
	file2, err := filesystem.ReadOnlyLock(lock.Name())
	assert.NilError(t, err, "acquiring another read-only lock should succeed")
	err = filesystem.Unlock(file)
	assert.NilError(t, err, "releasing a read-only lock should succeed")
	err = filesystem.Unlock(file2)
	assert.NilError(t, err, "releasing another read-only lock should succeed")
}

func TestLockWriteConcurrent(t *testing.T) {
	t.Parallel()

	var waitGroup sync.WaitGroup

	var concurrentKey uint32

	tempDir := t.TempDir()

	waitGroup.Add(2)

	// Start a lock, set the key, sleep 1s and confirm the key is still the same
	go func() {
		defer waitGroup.Done()

		lErr := filesystem.WithLock(tempDir, func() error {
			atomic.StoreUint32(&concurrentKey, routine1)

			time.Sleep(1 * time.Second)
			assert.Equal(t, atomic.LoadUint32(&concurrentKey), routine1)

			return nil
		})

		assert.NilError(t, lErr, "locking should not error")
	}()

	// Wait 0.5s, start another lock, set the key, sleep 1s and confirm the key is still the same
	go func() {
		defer waitGroup.Done()

		time.Sleep(500 * time.Millisecond)

		lErr := filesystem.WithLock(tempDir, func() error {
			atomic.StoreUint32(&concurrentKey, routine2)

			time.Sleep(1 * time.Second)
			assert.Equal(t, atomic.LoadUint32(&concurrentKey), routine2)

			return nil
		})

		assert.NilError(t, lErr, "locking should not error")
	}()

	// Start a lock, set the key, wait 1s, confirm the key is still the same
	lErr := filesystem.WithLock(tempDir, func() error {
		atomic.StoreUint32(&concurrentKey, mainroutine1)

		time.Sleep(1 * time.Second)
		assert.Equal(t, atomic.LoadUint32(&concurrentKey), mainroutine1)

		return nil
	})
	assert.NilError(t, lErr, "locking should not error")

	// Wait 0.75s, start a lock, set the key, sleep 1s, confirm the key is unchanged
	time.Sleep(750 * time.Millisecond)

	lErr = filesystem.WithLock(tempDir, func() error {
		atomic.StoreUint32(&concurrentKey, mainroutine2)

		time.Sleep(1 * time.Second)
		assert.Equal(t, atomic.LoadUint32(&concurrentKey), mainroutine2)

		return nil
	})

	assert.NilError(t, lErr, "locking should not error")

	waitGroup.Wait()
}

func TestLockMultiRead(t *testing.T) {
	t.Parallel()

	var waitGroup sync.WaitGroup

	var concurrentKey uint32

	tempDir := t.TempDir()

	waitGroup.Add(3)

	// Start a readonly lock immediately
	// Then wait 1s inside the lock - confirm the key got changed by the second read routine
	go func() {
		t.Log("Entering routine 1")

		defer waitGroup.Done()

		lErr := filesystem.WithReadOnlyLock(tempDir, func() error {
			t.Log("Entering routine 1 read lock")

			atomic.StoreUint32(&concurrentKey, routine1)

			time.Sleep(1 * time.Second)
			assert.Equal(t, atomic.LoadUint32(&concurrentKey), routine2)

			return nil
		})

		assert.NilError(t, lErr, "locking should not error")
	}()

	// Wait 0.5s before locking, then change the key
	go func() {
		t.Log("Entering routine 2")

		defer waitGroup.Done()

		time.Sleep(500 * time.Millisecond)

		lErr := filesystem.WithReadOnlyLock(tempDir, func() error {
			t.Log("Entering routine 2 read lock")

			atomic.StoreUint32(&concurrentKey, routine2)

			return nil
		})

		assert.NilError(t, lErr, "locking should not error")
	}()

	time.Sleep(50 * time.Millisecond)
	// Start a write lock, confirm we have waited for the read locks to finish, change the key
	go func() {
		t.Log("Entering routine 3")

		defer waitGroup.Done()

		lErr := filesystem.WithLock(tempDir, func() error {
			t.Log("Entering routine 3 write lock")
			assert.Equal(t, atomic.LoadUint32(&concurrentKey), routine2)

			atomic.StoreUint32(&concurrentKey, routine3)

			return nil
		})

		assert.NilError(t, lErr, "locking should not error")
	}()

	waitGroup.Wait()
}

func TestLockWriteBlocksRead(t *testing.T) {
	t.Parallel()

	var waitGroup sync.WaitGroup

	var concurrentKey uint32

	tempDir := t.TempDir()

	waitGroup.Add(2)

	// Start a lock, set the key, sleep 1s and confirm the key is still the same
	go func() {
		defer waitGroup.Done()

		lErr := filesystem.WithLock(tempDir, func() error {
			time.Sleep(1 * time.Second)

			atomic.StoreUint32(&concurrentKey, routine1)

			assert.Equal(t, atomic.LoadUint32(&concurrentKey), routine1)

			return nil
		})

		assert.NilError(t, lErr, "locking should not error")
	}()

	time.Sleep(50 * time.Millisecond)

	// Start a readonly lock immediately
	// Confirm the key has been set by the write lock
	go func() {
		defer waitGroup.Done()

		lErr := filesystem.WithReadOnlyLock(tempDir, func() error {
			assert.Equal(t, atomic.LoadUint32(&concurrentKey), routine1)

			return nil
		})

		assert.NilError(t, lErr, "locking should not error")
	}()

	waitGroup.Wait()
}
