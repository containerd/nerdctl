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

package checkpointutil

import (
	"fmt"
	"os"
	"path/filepath"
)

func GetCheckpointDir(checkpointDir, checkpointID, containerID string, create bool) (string, error) {
	checkpointAbsDir := filepath.Join(checkpointDir, checkpointID)
	stat, err := os.Stat(checkpointAbsDir)
	if create {
		switch {
		case err == nil && stat.IsDir():
			err = fmt.Errorf("checkpoint with name %s already exists for container %s", checkpointID, containerID)
		case err != nil && os.IsNotExist(err):
			err = os.MkdirAll(checkpointAbsDir, 0o700)
		case err != nil:
			err = fmt.Errorf("%s exists and is not a directory", checkpointAbsDir)
		}
	} else {
		switch {
		case err != nil:
			err = fmt.Errorf("checkpoint %s does not exist for container %s", checkpointID, containerID)
		case stat.IsDir():
			err = nil
		default:
			err = fmt.Errorf("%s exists and is not a directory", checkpointAbsDir)
		}
	}
	return checkpointAbsDir, err
}
