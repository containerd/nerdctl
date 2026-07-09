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

package ociruntimeutil

import (
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/containerd/containerd/v2/pkg/kernelversion"
)

var (
	rroCacheMu sync.Mutex
	rroCache   = make(map[string]error) // key: runtimeStr
)

// SupportsRecursivelyReadOnly returns nil when the kernel and the OCI runtime
// specified by runtimeStr (the value of the `--runtime` flag) support
// recursive read-only (RRO) bind mounts.
// The result is cached per runtimeStr for the lifetime of the process.
func SupportsRecursivelyReadOnly(runtimeStr string) error {
	rroCacheMu.Lock()
	defer rroCacheMu.Unlock()
	if err, ok := rroCache[runtimeStr]; ok {
		return err
	}
	err := supportsRecursivelyReadOnly(runtimeStr)
	rroCache[runtimeStr] = err
	return err
}

func supportsRecursivelyReadOnly(runtimeStr string) error {
	// Recursive read-only mounts (mount_setattr(2) with MOUNT_ATTR_RDONLY and
	// AT_RECURSIVE) require kernel >= 5.12.
	ok, err := kernelversion.GreaterEqualThan(kernelversion.KernelVersion{Kernel: 5, Major: 12})
	if err != nil {
		return fmt.Errorf("failed to detect whether the kernel supports recursive read-only mounts: %w", err)
	}
	if !ok {
		return errors.New("recursive read-only mounts require kernel >= 5.12")
	}
	binary, err := BinaryFromRuntimeStr(runtimeStr)
	if err != nil {
		return fmt.Errorf("failed to detect whether the OCI runtime supports recursive read-only mounts: %w", err)
	}
	f, err := Features(binary)
	if err != nil {
		return fmt.Errorf("failed to detect whether the OCI runtime %q supports recursive read-only mounts (hint: recursive read-only mounts require runc >= 1.1 or crun >= 1.8.6): %w",
			binary, err)
	}
	if !slices.Contains(f.MountOptions, "rro") {
		return fmt.Errorf("the OCI runtime %q does not support recursive read-only (\"rro\") mounts", binary)
	}
	return nil
}
