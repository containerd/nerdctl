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

package container

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"

	"github.com/containerd/nerdctl/v2/pkg/containerutil"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

// This is a separate set of tests for cp specifically meant to test corner or extreme cases that do not fit in the normal testing rig
// because of their complexity

func TestCopyAcid(t *testing.T) {
	t.Parallel()

	t.Run("Travelling along volumes w/o read-only", func(t *testing.T) {
		t.Parallel()
		testID := testutil.Identifier(t)
		tempDir := t.TempDir()
		base := testutil.NewBase(t)
		base.Dir = tempDir

		sourceFile := filepath.Join(tempDir, "hostfile")
		sourceFileContent := []byte(testID)

		roContainer := testID + "-ro"
		rwContainer := testID + "-rw"

		setup := func() {
			base.Cmd("volume", "create", testID+"-1-ro").AssertOK()
			base.Cmd("volume", "create", testID+"-2-rw").AssertOK()
			base.Cmd("volume", "create", testID+"-3-rw").AssertOK()
			base.Cmd("run", "-d", "-w", containerCwd, "--name", roContainer, "--read-only",
				"-v", fmt.Sprintf("%s:%s:ro", testID+"-1-ro", "/vol1/dir1/ro"),
				"-v", fmt.Sprintf("%s:%s", testID+"-2-rw", "/vol2/dir2/rw"),
				testutil.CommonImage, "sleep", "Inf",
			).AssertOK()
			base.Cmd("run", "-d", "-w", containerCwd, "--name", rwContainer,
				"-v", fmt.Sprintf("%s:%s:ro", testID+"-1-ro", "/vol1/dir1/ro"),
				"-v", fmt.Sprintf("%s:%s", testID+"-3-rw", "/vol3/dir3/rw"),
				testutil.CommonImage, "sleep", "Inf",
			).AssertOK()

			base.Cmd("exec", rwContainer, "sh", "-euxc", "cd /vol3/dir3/rw; ln -s ../../../ relativelinktoroot").AssertOK()
			base.Cmd("exec", rwContainer, "sh", "-euxc", "cd /vol3/dir3/rw; ln -s / absolutelinktoroot").AssertOK()
			base.Cmd("exec", roContainer, "sh", "-euxc", "cd /vol2/dir2/rw; ln -s ../../../ relativelinktoroot").AssertOK()
			base.Cmd("exec", roContainer, "sh", "-euxc", "cd /vol2/dir2/rw; ln -s / absolutelinktoroot").AssertOK()
			// Create file on the host
			err := os.WriteFile(sourceFile, sourceFileContent, filePerm)
			assert.NilError(t, err)
		}

		tearDown := func() {
			base.Cmd("rm", "-f", roContainer).Run()
			base.Cmd("rm", "-f", rwContainer).Run()
			base.Cmd("volume", "rm", testID+"-1-ro").Run()
			base.Cmd("volume", "rm", testID+"-2-rw").Run()
			base.Cmd("volume", "rm", testID+"-3-rw").Run()
		}

		t.Cleanup(tearDown)
		tearDown()

		setup()

		expectedErr := containerutil.ErrTargetIsReadOnly.Error()
		if testutil.GetTarget() == testutil.Docker {
			expectedErr = ""
		}

		t.Run("Cannot copy into a read-only root", func(t *testing.T) {
			t.Parallel()

			base.Cmd("cp", sourceFile, roContainer+":/").Assert(icmd.Expected{
				ExitCode: 1,
				Err:      expectedErr,
			})
		})

		t.Run("Cannot copy into a read-only mount, in a rw container", func(t *testing.T) {
			t.Parallel()

			base.Cmd("cp", sourceFile, rwContainer+":/vol1/dir1/ro").Assert(icmd.Expected{
				ExitCode: 1,
				Err:      expectedErr,
			})
		})

		t.Run("Can copy into a read-write mount in a read-only container", func(t *testing.T) {
			t.Parallel()

			base.Cmd("cp", sourceFile, roContainer+":/vol2/dir2/rw").Assert(icmd.Expected{
				ExitCode: 0,
			})
		})

		t.Run("Traverse read-only locations to a read-write location", func(t *testing.T) {
			t.Parallel()

			base.Cmd("cp", sourceFile, roContainer+":/vol1/dir1/ro/../../../vol2/dir2/rw").Assert(icmd.Expected{
				ExitCode: 0,
			})
		})

		t.Run("Follow an absolute symlink inside a read-write mount to a read-only root", func(t *testing.T) {
			t.Parallel()

			base.Cmd("cp", sourceFile, roContainer+":/vol2/dir2/rw/absolutelinktoroot").Assert(icmd.Expected{
				ExitCode: 1,
				Err:      expectedErr,
			})
		})

		t.Run("Follow am absolute symlink inside a read-write mount to a read-only mount", func(t *testing.T) {
			t.Parallel()

			base.Cmd("cp", sourceFile, rwContainer+":/vol3/dir3/rw/absolutelinktoroot/vol1/dir1/ro").Assert(icmd.Expected{
				ExitCode: 1,
				Err:      expectedErr,
			})
		})

		t.Run("Follow a relative symlink inside a read-write location to a read-only root", func(t *testing.T) {
			t.Parallel()

			base.Cmd("cp", sourceFile, roContainer+":/vol2/dir2/rw/relativelinktoroot").Assert(icmd.Expected{
				ExitCode: 1,
				Err:      expectedErr,
			})
		})

		t.Run("Follow a relative symlink inside a read-write location to a read-only mount", func(t *testing.T) {
			t.Parallel()

			base.Cmd("cp", sourceFile, rwContainer+":/vol3/dir3/rw/relativelinktoroot/vol1/dir1/ro").Assert(icmd.Expected{
				ExitCode: 1,
				Err:      expectedErr,
			})
		})

		t.Run("Cannot copy into a HOST read-only location", func(t *testing.T) {
			t.Parallel()

			// Root will just ignore the 000 permission on the host directory.
			if !rootlessutil.IsRootless() {
				t.Skip("This test does not work rootful")
			}

			err := os.MkdirAll(filepath.Join(tempDir, "rotest"), 0o000)
			assert.NilError(t, err)
			base.Cmd("cp", roContainer+":/etc/issue", filepath.Join(tempDir, "rotest")).Assert(icmd.Expected{
				ExitCode: 1,
				Err:      expectedErr,
			})
		})

	})
}
