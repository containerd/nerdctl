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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/containerutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

// This is a separate set of tests for cp specifically meant to test corner or extreme cases that do not fit in the normal testing rig
// because of their complexity

func TestCopyAcid(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		testID := data.Identifier()
		tempDir := t.TempDir()

		sourceFile := filepath.Join(tempDir, "hostfile")
		sourceFileContent := []byte(testID)

		roContainer := testID + "-ro"
		rwContainer := testID + "-rw"

		data.Labels().Set("sourceFile", sourceFile)
		data.Labels().Set("sourceFileContent", string(sourceFileContent))
		data.Labels().Set("roContainer", roContainer)
		data.Labels().Set("rwContainer", rwContainer)

		helpers.Ensure("volume", "create", testID+"-1-ro")
		helpers.Ensure("volume", "create", testID+"-2-ro")
		helpers.Ensure("volume", "create", testID+"-3-ro")

		helpers.Ensure("run", "-d", "-w", containerCwd, "--name", roContainer, "--read-only",
			"-v", fmt.Sprintf("%s:%s:ro", testID+"-1-ro", "/vol1/dir1/ro"),
			"-v", fmt.Sprintf("%s:%s", testID+"-2-rw", "/vol2/dir2/rw"),
			testutil.CommonImage, "sleep", nerdtest.Infinity,
		)
		nerdtest.EnsureContainerStarted(helpers, roContainer)

		helpers.Ensure("run", "-d", "-w", containerCwd, "--name", rwContainer,
			"-v", fmt.Sprintf("%s:%s:ro", testID+"-1-ro", "/vol1/dir1/ro"),
			"-v", fmt.Sprintf("%s:%s", testID+"-3-rw", "/vol3/dir3/rw"),
			testutil.CommonImage, "sleep", nerdtest.Infinity,
		)
		nerdtest.EnsureContainerStarted(helpers, rwContainer)

		helpers.Ensure("exec", rwContainer, "sh", "-euxc", "cd /vol3/dir3/rw; ln -s ../../../ relativelinktoroot")
		helpers.Ensure("exec", rwContainer, "sh", "-euxc", "cd /vol3/dir3/rw; ln -s / absolutelinktoroot")
		helpers.Ensure("exec", roContainer, "sh", "-euxc", "cd /vol2/dir2/rw; ln -s ../../../ relativelinktoroot")
		helpers.Ensure("exec", roContainer, "sh", "-euxc", "cd /vol2/dir2/rw; ln -s / absolutelinktoroot")

		// Create file on the host
		err := os.WriteFile(sourceFile, sourceFileContent, filePerm)
		assert.NilError(t, err)

		expectedErr := containerutil.ErrTargetIsReadOnly.Error()
		if nerdtest.IsDocker() {
			expectedErr = ""
		}
		data.Labels().Set("expectedErr", expectedErr)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		testID := data.Identifier()
		helpers.Anyhow("rm", "-f", testID+"-ro")
		helpers.Anyhow("rm", "-f", testID+"-rw")
		helpers.Anyhow("volume", "rm", testID+"-1-ro")
		helpers.Anyhow("volume", "rm", testID+"-2-rw")
		helpers.Anyhow("volume", "rm", testID+"-3-rw")
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "Cannot copy into a read-only root",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("cp", data.Labels().Get("sourceFile"), data.Labels().Get("roContainer")+":/")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New(data.Labels().Get("expectedErr"))},
				}
			},
		},
		{
			Description: "Cannot copy into a read-only mount, in a rw container",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("cp", data.Labels().Get("sourceFile"), data.Labels().Get("rwContainer")+":/vol1/dir1/ro")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New(data.Labels().Get("expectedErr"))},
				}
			},
		},
		{
			Description: "Can copy into a read-write mount in a read-only container",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("cp", data.Labels().Get("sourceFile"), data.Labels().Get("roContainer")+":/vol2/dir2/rw")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
				}
			},
		},
		{
			Description: "Traverse read-only locations to a read-write location",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("cp", data.Labels().Get("sourceFile"), data.Labels().Get("roContainer")+":/vol1/dir1/ro/../../../vol2/dir2/rw")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
				}
			},
		},
		{
			Description: "Follow an absolute symlink inside a read-write mount to a read-only root",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("cp", data.Labels().Get("sourceFile"), data.Labels().Get("roContainer")+":/vol2/dir2/rw/absolutelinktoroot")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New(data.Labels().Get("expectedErr"))},
				}
			},
		},
		{
			Description: "Follow am absolute symlink inside a read-write mount to a read-only mount",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("cp", data.Labels().Get("sourceFile"), data.Labels().Get("rwContainer")+":/vol3/dir3/rw/absolutelinktoroot/vol1/dir1/ro")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New(data.Labels().Get("expectedErr"))},
				}
			},
		},
		{
			Description: "Follow a relative symlink inside a read-write location to a read-only root",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("cp", data.Labels().Get("sourceFile"), data.Labels().Get("roContainer")+":/vol2/dir2/rw/relativelinktoroot")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New(data.Labels().Get("expectedErr"))},
				}
			},
		},
		{
			Description: "Follow a relative symlink inside a read-write location to a read-only mount",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("cp", data.Labels().Get("sourceFile"), data.Labels().Get("rwContainer")+":/vol3/dir3/rw/relativelinktoroot/vol1/dir1/ro")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New(data.Labels().Get("expectedErr"))},
				}
			},
		},
		{
			Description: "Cannot copy into a HOST read-only location",
			Require:     nerdtest.Rootless,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				tempDir := t.TempDir()
				err := os.MkdirAll(filepath.Join(tempDir, "rotest"), 0o000)
				assert.NilError(t, err)
				return helpers.Command("cp", data.Labels().Get("roContainer")+":/etc/issue", filepath.Join(tempDir, "rotest"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New(data.Labels().Get("expectedErr"))},
				}
			},
		},
	}

	testCase.Run(t)
}
