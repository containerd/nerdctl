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
	"errors"
	"os"
)

func ReadFile(filename string) (data []byte, err error) {
	if err = ensureRecovery(filename); err != nil {
		return nil, err
	}

	data, err = os.ReadFile(filename)
	if err != nil {
		return nil, errors.Join(ErrFilesystemFailure, err)
	}

	return data, nil
}

func Stat(filename string) (os.FileInfo, error) {
	if err := ensureRecovery(filename); err != nil {
		return nil, errors.Join(ErrFilesystemFailure, err)
	}

	return os.Stat(filename)
}

// WriteFile implements an atomic and durable alternative to os.WriteFile that does not change inodes (unlike the usual
// approach on atomic writes that relies on renaming files).
func WriteFile(filename string, data []byte, perm os.FileMode) error {
	_, err := WriteFileWithRollback(filename, data, perm)
	return err
}
