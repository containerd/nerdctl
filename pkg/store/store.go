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

// Package store provides a concurrency-safe lightweight storage solution with a simple interface.
// Embedders should call `Lock` and `defer Release` (or WithLock(func()error)) to wrap operations,
// or series of operations, to ensure secure use.
// Furthermore, a Store implementation must do atomic writes, providing guarantees that interrupted partial writes
// never get committed.
// The Store interface itself is meant to be generic, and alternative stores (memory based, or content-addressable)
// may be implemented that satisfies it.
// This package also provides the default, file based implementation that we are using.
package store

import (
	"errors"

	"github.com/containerd/errdefs"
)

var (
	// ErrInvalidArgument may be returned by Get, Set, List, or Delete by specific SafeStore implementations
	// (eg: filesystem), when they want to impose implementation dependent restrictions on the identifiers
	// (filesystems typically do).
	ErrInvalidArgument = errdefs.ErrInvalidArgument
	// ErrNotFound may be returned by Get or Delete when the requested key is not present in the store
	ErrNotFound = errdefs.ErrNotFound
	// ErrSystemFailure may be returned by implementations when an internal failure occurs.
	// For example, for a filesystem implementation, failure to create a file will be wrapped by this error.
	ErrSystemFailure = errors.New("system failure")
	// ErrLockFailure may be returned by ReadLock, WriteLock, or Unlock, when the underlying locking mechanism fails.
	// In the case of the filesystem implementation, inability to lock the directory will return it.
	ErrLockFailure = errors.New("lock failure")
	// ErrFaultyImplementation may be returned by Get or Set when the target key exists and is a dir,
	// or by List when the target key is a file
	// This is indicative the code using the store is not consistent with what it treats as group, and what it treats as key
	// and is definitely a bug in that code
	// Missing lock will also trigger this when detected.
	ErrFaultyImplementation = errors.New("code needs to be fixed")
)

// Store represents a store that is able to grant an exclusive lock (ensuring concurrency safety,
// both between go routines and across multiple binaries invocations), and is performing atomic operations.
// Note that Store allows manipulating nested objects:
// - Set([]byte("mykeyvalue"), "group", "subgroup", "my key1")
// - Set([]byte("mykeyvalue"), "group", "subgroup", "my key2")
// - Get("group", "subgroup", "my key1")
// - List("group", "subgroup")
// Note that both Delete and Exists can be applied indifferently to specific keys, or groups.
type Store interface {
	Locker
	Manager
}

// Manager describes operations that can be performed on the store
type Manager interface {
	// List will return a slice of all subgroups (eg: subdirectories), or keys (eg: files), under a specific group (eg: dir)
	// Note that `key...` may be omitted, in which case, all objects' names at the root of the store are returned.
	// Example, in the volumestore, List() will return all existing volumes names
	List(key ...string) ([]string, error)
	// Exists checks that a given key exists
	// Example: Exists("meta.json")
	Exists(key ...string) (bool, error)
	// Get returns the content of a key
	Get(key ...string) ([]byte, error)
	// Set saves bytes to a key
	Set(data []byte, key ...string) error
	// Delete removes a key or a group
	Delete(key ...string) error
	// Location returns the absolute path to a certain resource
	// Note that this technically "leaks" (filesystem) implementation details up.
	// It is necessary though when we are going to pass these filepath to containerd for eg.
	Location(key ...string) (string, error)

	// GroupSize will return the combined size of all objects stored under the group (eg: dir)
	GroupSize(key ...string) (int64, error)
	// GroupEnsure ensures that a given group (eg: directory) exists
	GroupEnsure(key ...string) error
}

// Locker describes a locking mechanism that can be used to encapsulate operations in a safe way
type Locker interface {
	Lock() error
	Release() error
	WithLock(fun func() error) (err error)
}
