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
	"runtime"
	"strconv"
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
	if runtime.GOOS == "windows" {
		t.Skip("export is not supported on Windows")
	}

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
	if runtime.GOOS == "windows" {
		t.Skip("export is not supported on Windows")
	}

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

func TestExportReplacesExistingFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("export is not supported on Windows")
	}

	testCase := nerdtest.Setup()
	testCase.NoParallel = true

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		bigger := data.Identifier("bigger")
		smaller := data.Identifier("smaller")
		outFile := filepath.Join(data.Temp().Path(), "reused.tar")

		helpers.Ensure("create", "--name", bigger, testutil.NginxAlpineImage)
		helpers.Ensure("create", "--name", smaller, testutil.CommonImage)

		// The bigger filesystem first, so that the smaller one written over it has something to
		// leave behind.
		helpers.Ensure("export", "-o", outFile, bigger)
		info, err := os.Stat(outFile)
		assert.NilError(t, err)

		data.Labels().Set("bigger", bigger)
		data.Labels().Set("smaller", smaller)
		data.Labels().Set("outFile", outFile)
		data.Labels().Set("biggerSize", strconv.FormatInt(info.Size(), 10))
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Labels().Get("bigger"))
		helpers.Anyhow("rm", "-f", data.Labels().Get("smaller"))
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("export", "-o", data.Labels().Get("outFile"), data.Labels().Get("smaller"))
	}
	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0,
			Output: func(stdout string, t tig.T) {
				info, err := os.Stat(data.Labels().Get("outFile"))
				assert.NilError(t, err)
				biggerSize, err := strconv.ParseInt(data.Labels().Get("biggerSize"), 10, 64)
				assert.NilError(t, err)

				// The file must hold the smaller archive and nothing else. A tar reader stops at
				// the end-of-archive marker, so a tail left over from the bigger archive would go
				// unnoticed on read, but the file would still carry the bytes of another
				// container.
				assert.Assert(t, info.Size() < biggerSize,
					"expected the file to shrink to the new archive, still %d of %d bytes",
					info.Size(), biggerSize)
			},
		}
	}

	testCase.Run(t)
}

func TestExportNonexistentContainer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("export is not supported on Windows")
	}

	testCase := nerdtest.Setup()
	testCase.Command = test.Command("export", "nonexistent-container")
	testCase.Expected = test.Expects(1, nil, nil)

	testCase.Run(t)
}
