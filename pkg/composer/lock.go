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

package composer

import (
	"os"

	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/internal/filesystem"
)

//nolint:unused
var locked *os.File

func Lock(dataRoot string, address string) error {
	// Compose right now cannot be made safe to use concurrently, as we shell out to nerdctl for multiple operations,
	// preventing us from using the lock mechanisms from the API.
	// This here allows to impose a global lock, effectively preventing multiple compose commands from being run in parallel and
	// preventing some of the problems with concurrent execution.
	// This should be removed once we have better, in-depth solutions to make compose concurrency safe.
	// Note that in most cases we do not close the lock explicitly. Instead, the lock will get released when the `locked` global
	// variable will get collected and the file descriptor closed (eg: when the binary exits).
	var err error
	dataStore, err := clientutil.DataStore(dataRoot, address)
	if err != nil {
		return err
	}
	locked, err = filesystem.Lock(dataStore)
	return err
}

func Unlock() error {
	return filesystem.Unlock(locked)
}
