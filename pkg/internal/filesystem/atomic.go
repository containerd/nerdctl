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
	"os"
	"path/filepath"
)

func AtomicWrite(parent string, fileName string, perm os.FileMode, data []byte) error {
	dest := filepath.Join(parent, fileName)
	temp := filepath.Join(parent, ".temp."+fileName)

	err := os.WriteFile(temp, data, perm)
	if err != nil {
		return err
	}

	err = os.Rename(temp, dest)
	if err != nil {
		return err
	}

	return nil
}
