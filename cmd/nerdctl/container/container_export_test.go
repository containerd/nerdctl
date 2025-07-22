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
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

// validateExportedTar checks that the tar file exists and contains /bin/busybox
func validateExportedTar(outFile string) test.Comparator {
	return func(stdout string, t tig.T) {
		// Check if the tar file was created
		_, err := os.Stat(outFile)
		assert.Assert(t, !os.IsNotExist(err), "exported tar file %s was not created", outFile)

		// Open and read the tar file to check for /bin/busybox
		file, err := os.Open(outFile)
		assert.NilError(t, err, "failed to open tar file %s", outFile)
		defer file.Close()

		tarReader := tar.NewReader(file)
		busyboxFound := false

		for {
			header, err := tarReader.Next()
			if err == io.EOF {
				break
			}
			assert.NilError(t, err, "failed to read tar entry")

			if header.Name == "bin/busybox" || header.Name == "./bin/busybox" {
				busyboxFound = true
				break
			}
		}

		assert.Assert(t, busyboxFound, "exported tar file %s does not contain /bin/busybox", outFile)
		t.Log("Export validation passed: tar file exists and contains /bin/busybox")
	}
}

func TestExportStoppedContainer(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		identifier := data.Identifier("container")
		helpers.Ensure("create", "--name", identifier, testutil.CommonImage)
		data.Labels().Set("cID", identifier)
		data.Labels().Set("outFile", filepath.Join(os.TempDir(), identifier+".tar"))
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("container", "rm", "-f", data.Labels().Get("cID"))
		helpers.Anyhow("rm", "-f", data.Labels().Get("cID"))
		os.Remove(data.Labels().Get("outFile"))
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "export command succeeds",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("export", "-o", data.Labels().Get("outFile"), data.Labels().Get("cID"))
			},
			Expected: test.Expects(0, nil, nil),
		},
		{
			Description: "tar file exists and has content",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				// Use a simple command that always succeeds to trigger the validation
				return helpers.Custom("echo", "validating tar file")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output:   validateExportedTar(data.Labels().Get("outFile")),
				}
			},
		},
	}

	testCase.Run(t)
}

func TestExportRunningContainer(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		identifier := data.Identifier("container")
		helpers.Ensure("run", "-d", "--name", identifier, testutil.CommonImage, "sleep", nerdtest.Infinity)
		data.Labels().Set("cID", identifier)
		data.Labels().Set("outFile", filepath.Join(os.TempDir(), identifier+".tar"))
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Labels().Get("cID"))
		os.Remove(data.Labels().Get("outFile"))
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "export command succeeds",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("export", "-o", data.Labels().Get("outFile"), data.Labels().Get("cID"))
			},
			Expected: test.Expects(0, nil, nil),
		},
		{
			Description: "tar file exists and has content",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				// Use a simple command that always succeeds to trigger the validation
				return helpers.Custom("echo", "validating tar file")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output:   validateExportedTar(data.Labels().Get("outFile")),
				}
			},
		},
	}

	testCase.Run(t)
}

func TestExportNonexistentContainer(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Command = test.Command("export", "nonexistent-container")
	testCase.Expected = test.Expects(1, nil, nil)

	testCase.Run(t)
}
