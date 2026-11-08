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

package checkpoint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveOrphanedCheckpointDeletesExistingDir(t *testing.T) {
	dir := t.TempDir()
	checkpoint := filepath.Join(dir, "cp1")
	if err := os.Mkdir(checkpoint, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkpoint, "dump.log"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := removeOrphanedCheckpoint(dir, "cp1", "gone-container"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(checkpoint); !os.IsNotExist(err) {
		t.Fatalf("checkpoint directory still present: %v", err)
	}
}

func TestRemoveOrphanedCheckpointReportsMissingContainerWhenDirIsGone(t *testing.T) {
	err := removeOrphanedCheckpoint(t.TempDir(), "missing", "gone-container")
	if err == nil || !strings.Contains(err.Error(), "no such container") {
		t.Fatalf("got %v", err)
	}
}
