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

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestComposeCopy(t *testing.T) {
	base := testutil.NewBase(t)

	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

services:
  svc0:
    image: %s
    command: "sleep infinity"
`, testutil.CommonImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").AssertOK()

	// gernetate test file
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "test-file")
	srcFileContent := []byte("test-file-content")
	err := os.WriteFile(srcFile, srcFileContent, 0o644)
	assert.NilError(t, err)

	// test copy to service
	destPath := "/dest-no-exist-no-slash"
	base.ComposeCmd("-f", comp.YAMLFullPath(), "cp", srcFile, "svc0:"+destPath).AssertOK()

	// test copy from service
	destFile := filepath.Join(srcDir, "test-file2")
	base.ComposeCmd("-f", comp.YAMLFullPath(), "cp", "svc0:"+destPath, destFile).AssertOK()

	destFileContent, err := os.ReadFile(destFile)
	assert.NilError(t, err)
	assert.DeepEqual(t, srcFileContent, destFileContent)

}
