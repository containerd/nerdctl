/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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

package volumestore

import (
	"os"
	"path/filepath"

	"github.com/containerd/containerd/errdefs"
)

// Path returns a string like `/var/lib/nerdctl/1935db59/volumes/default`.
func Path(dataStore, ns string) (string, error) {
	if dataStore == "" || ns == "" {
		return "", errdefs.ErrInvalidArgument
	}
	volStore := filepath.Join(dataStore, "volumes", ns)
	return volStore, nil
}

// New returns a string like `/var/lib/nerdctl/1935db59/volumes/default`.
// The returned directory is guaranteed to exist.
func New(dataStore, ns string) (string, error) {
	volStore, err := Path(dataStore, ns)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(volStore, 0700); err != nil {
		return "", err
	}
	return volStore, nil
}

// DataDirName is "_data"
const DataDirName = "_data"
