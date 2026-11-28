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

package cioutil

import "path/filepath"

// stdinFIFOName is the file name of a container's stdin FIFO inside its data
// store directory.
const stdinFIFOName = "stdin.fifo"

// StdinFIFOPath returns the stable path of a container's stdin FIFO.
//
// containerd generates a fresh FIFO directory per task, so those paths change
// across restarts and cannot be derived by another process. nerdctl pins stdin
// to this path instead, so that the process owning the container's stdio can
// find it from the data store alone.
//
// The file is left in place when the container stops, and goes away with the
// container's data store directory. Nothing reads meaning into its presence:
// whether the running task has stdin is established by opening it, in
// pkg/logging/broker_unix.go.
func StdinFIFOPath(dataStore, ns, id string) string {
	return filepath.Join(dataStore, "containers", ns, id, stdinFIFOName)
}
