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
	"strings"
	"syscall"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
)

func TestCopyToContainer(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	testContainer := testutil.Identifier(t)
	fmt.Printf("test container name: %s", testContainer)
	testStoppedContainer := testutil.Identifier(t)
	fmt.Printf("test stopped container name: %s", testStoppedContainer)

	base.Cmd("run", "-d", "--name", testStoppedContainer, testutil.CommonImage, "sleep", "1h").AssertOK()
	defer base.Cmd("rm", "-f", testStoppedContainer).Run()

	base.Cmd("run", "--name", testContainer, testutil.CommonImage).AssertOK()

	srcUID := os.Geteuid()
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "test-file")
	srcFileContent := []byte("test-file-content")
	err := os.WriteFile(srcFile, srcFileContent, 0644)
	assert.NilError(t, err)

	assertCat := func(catPath string, testContainer string, stopped bool) {
		if stopped {
			base.Cmd("start", testStoppedContainer)
		}
		t.Logf("catPath=%q", catPath)
		base.Cmd("exec", testContainer, "cat", catPath).AssertOutExactly(string(srcFileContent))
		base.Cmd("exec", testContainer, "stat", "-c", "%u", catPath).AssertOutExactly(fmt.Sprintf("%d\n", srcUID))
	}

	// For the test matrix, see https://docs.docker.com/engine/reference/commandline/cp/
	t.Run("SRC_PATH specifies a file", func(t *testing.T) {
		srcPath := srcFile
		t.Run("DEST_PATH does not exist", func(t *testing.T) {
			destPath := "/dest-no-exist-no-slash"
			base.Cmd("cp", srcPath, testContainer+":"+destPath).AssertOK()
			catPath := destPath
			assertCat(catPath, testContainer, false)
			base.Cmd("cp", srcPath, testStoppedContainer+":"+destPath).AssertOK()
			assertCat(catPath, testStoppedContainer, true)
		})
		t.Run("DEST_PATH does not exist and ends with /", func(t *testing.T) {
			destPath := "/dest-no-exist-with-slash/"
			base.Cmd("cp", srcPath, testContainer+":"+destPath).AssertFail()
			base.Cmd("cp", srcPath, testStoppedContainer+":"+destPath).AssertFail()
		})
		t.Run("DEST_PATH exists and is a file", func(t *testing.T) {
			destPath := "/dest-file-exists"
			base.Cmd("exec", testContainer, "touch", destPath).AssertOK()
			base.Cmd("cp", srcPath, testContainer+":"+destPath).AssertOK()
			catPath := destPath
			assertCat(catPath, testContainer, false)
			base.Cmd("cp", srcPath, testStoppedContainer+":"+destPath).AssertOK()
			assertCat(catPath, testStoppedContainer, true)
		})
		t.Run("DEST_PATH exists and is a directory", func(t *testing.T) {
			destPath := "/dest-dir-exists"
			base.Cmd("exec", testContainer, "mkdir", "-p", destPath).AssertOK()
			base.Cmd("cp", srcPath, testContainer+":"+destPath).AssertOK()
			catPath := filepath.Join(destPath, filepath.Base(srcFile))
			assertCat(catPath, testStoppedContainer, false)
			base.Cmd("start", testStoppedContainer).AssertOK()
			base.Cmd("exec", testStoppedContainer, "mkdir", "-p", destPath).AssertOK()
			base.Cmd("stop", testStoppedContainer).AssertOK()
			base.Cmd("cp", srcPath, testStoppedContainer+":"+destPath).AssertOK()
			assertCat(catPath, testStoppedContainer, true)
		})
	})
	t.Run("SRC_PATH specifies a directory", func(t *testing.T) {
		srcPath := srcDir
		t.Run("DEST_PATH does not exist", func(t *testing.T) {
			destPath := "/dest2-no-exist"
			base.Cmd("cp", srcPath, testContainer+":"+destPath).AssertOK()
			catPath := filepath.Join(destPath, filepath.Base(srcFile))
			assertCat(catPath, testContainer, false)
			base.Cmd("cp", srcPath, testStoppedContainer+":"+destPath).AssertOK()
			assertCat(catPath, testStoppedContainer, true)
		})
		t.Run("DEST_PATH exists and is a file", func(t *testing.T) {
			destPath := "/dest2-file-exists"
			base.Cmd("exec", testContainer, "touch", destPath).AssertOK()
			base.Cmd("cp", srcPath, testContainer+":"+destPath).AssertFail()
			base.Cmd("start", testStoppedContainer).AssertOK()
			base.Cmd("exec", testStoppedContainer, "touch", destPath).AssertOK()
			base.Cmd("stop", testStoppedContainer).AssertOK()
			base.Cmd("cp", srcPath, testStoppedContainer+":"+destPath).AssertFail()
		})
		t.Run("DEST_PATH exists and is a directory", func(t *testing.T) {
			t.Run("SRC_PATH does not end with `/.`", func(t *testing.T) {
				destPath := "/dest2-dir-exists"
				base.Cmd("exec", testContainer, "mkdir", "-p", destPath).AssertOK()
				base.Cmd("cp", srcPath, testContainer+":"+destPath).AssertOK()
				catPath := filepath.Join(destPath, strings.TrimPrefix(srcFile, filepath.Dir(srcDir)+"/"))
				assertCat(catPath, testContainer, false)
				base.Cmd("start", testStoppedContainer).AssertOK()
				base.Cmd("exec", testStoppedContainer, "mkdir", "-p", destPath).AssertOK()
				base.Cmd("cp", srcPath, testStoppedContainer+":"+destPath).AssertOK()
				assertCat(catPath, testStoppedContainer, true)
			})
			t.Run("SRC_PATH does end with `/.`", func(t *testing.T) {
				srcPath += "/."
				destPath := "/dest2-dir2-exists"
				base.Cmd("exec", testContainer, "mkdir", "-p", destPath).AssertOK()
				base.Cmd("cp", srcPath, testContainer+":"+destPath).AssertOK()
				catPath := filepath.Join(destPath, filepath.Base(srcFile))
				t.Logf("catPath=%q", catPath)
				assertCat(catPath, testContainer, false)
				base.Cmd("start", testStoppedContainer).AssertOK()
				base.Cmd("exec", testStoppedContainer, "mkdir", "-p", destPath).AssertOK()
				base.Cmd("cp", srcPath, testContainer+":"+destPath).AssertOK()
				assertCat(catPath, testStoppedContainer, true)

			})
		})
	})
}

func TestCopyFromContainer(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	testContainer := testutil.Identifier(t)

	base.Cmd("run", "-d", "--name", testContainer, testutil.CommonImage, "sleep", "1h").AssertOK()
	defer base.Cmd("rm", "-f", testContainer).Run()

	euid := os.Geteuid()
	srcUID := 42
	srcDir := "/test-dir"
	srcFile := filepath.Join(srcDir, "test-file")
	srcFileContent := []byte("test-file-content")
	mkSrcScript := fmt.Sprintf("mkdir -p %q && echo -n %q >%q && chown %d %q", srcDir, srcFileContent, srcFile, srcUID, srcFile)
	base.Cmd("exec", testContainer, "sh", "-euc", mkSrcScript).AssertOK()

	assertCat := func(catPath string) {
		t.Logf("catPath=%q", catPath)
		got, err := os.ReadFile(catPath)
		assert.NilError(t, err)
		assert.DeepEqual(t, srcFileContent, got)
		st, err := os.Stat(catPath)
		assert.NilError(t, err)
		stSys := st.Sys().(*syscall.Stat_t)
		// stSys.Uid matches euid, not srcUID
		assert.DeepEqual(t, uint32(euid), stSys.Uid)
	}

	td := t.TempDir()
	// For the test matrix, see https://docs.docker.com/engine/reference/commandline/cp/
	t.Run("SRC_PATH specifies a file", func(t *testing.T) {
		srcPath := srcFile
		t.Run("DEST_PATH does not exist", func(t *testing.T) {
			destPath := filepath.Join(td, "dest-no-exist-no-slash")
			base.Cmd("cp", testContainer+":"+srcPath, destPath).AssertOK()
			catPath := destPath
			assertCat(catPath)
		})
		t.Run("DEST_PATH does not exist and ends with /", func(t *testing.T) {
			destPath := td + "/dest-no-exist-with-slash/" // Avoid filepath.Join, to forcibly append "/"
			base.Cmd("cp", testContainer+":"+srcPath, destPath).AssertFail()
		})
		t.Run("DEST_PATH exists and is a file", func(t *testing.T) {
			destPath := filepath.Join(td, "dest-file-exists")
			err := os.WriteFile(destPath, []byte(""), 0644)
			assert.NilError(t, err)
			base.Cmd("cp", testContainer+":"+srcPath, destPath).AssertOK()
			catPath := destPath
			assertCat(catPath)
		})
		t.Run("DEST_PATH exists and is a directory", func(t *testing.T) {
			destPath := filepath.Join(td, "dest-dir-exists")
			err := os.Mkdir(destPath, 0755)
			assert.NilError(t, err)
			base.Cmd("cp", testContainer+":"+srcPath, destPath).AssertOK()
			catPath := filepath.Join(destPath, filepath.Base(srcFile))
			assertCat(catPath)
		})
	})
	t.Run("SRC_PATH specifies a directory", func(t *testing.T) {
		srcPath := srcDir
		t.Run("DEST_PATH does not exist", func(t *testing.T) {
			destPath := filepath.Join(td, "dest2-no-exist")
			base.Cmd("cp", testContainer+":"+srcPath, destPath).AssertOK()
			catPath := filepath.Join(destPath, filepath.Base(srcFile))
			assertCat(catPath)
		})
		t.Run("DEST_PATH exists and is a file", func(t *testing.T) {
			destPath := filepath.Join(td, "dest2-file-exists")
			err := os.WriteFile(destPath, []byte(""), 0644)
			assert.NilError(t, err)
			base.Cmd("cp", srcPath, testContainer+":"+destPath).AssertFail()
		})
		t.Run("DEST_PATH exists and is a directory", func(t *testing.T) {
			t.Run("SRC_PATH does not end with `/.`", func(t *testing.T) {
				destPath := filepath.Join(td, "dest2-dir-exists")
				err := os.Mkdir(destPath, 0755)
				assert.NilError(t, err)
				base.Cmd("cp", testContainer+":"+srcPath, destPath).AssertOK()
				catPath := filepath.Join(destPath, strings.TrimPrefix(srcFile, filepath.Dir(srcDir)+"/"))
				assertCat(catPath)
			})
			t.Run("SRC_PATH does end with `/.`", func(t *testing.T) {
				srcPath += "/."
				destPath := filepath.Join(td, "dest2-dir2-exists")
				err := os.Mkdir(destPath, 0755)
				assert.NilError(t, err)
				base.Cmd("cp", testContainer+":"+srcPath, destPath).AssertOK()
				catPath := filepath.Join(destPath, filepath.Base(srcFile))
				assertCat(catPath)
			})
		})
	})
}
