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
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"

	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/containerutil"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/tarutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

// For the test matrix, see https://docs.docker.com/engine/reference/commandline/cp/
// Obviously, none of this is fully windows ready - obviously `nerdctl cp` itself is not either, so, ok for now.
const (
	// Use this to poke the testing rig for improper path handling
	// TODO: fuzz this more seriously
	// FIXME: the following will break the test (anything that will evaluate on the shell, obviously):
	// - `
	// - $a, ${a}, etc
	complexify = "" //  = "-~a0-_.(){}[]*#! \"'∞"

	pathDoesNotExistRelative = "does-not-exist" + complexify
	pathDoesNotExistAbsolute = string(os.PathSeparator) + "does-not-exist" + complexify
	pathIsAFileRelative      = "is-a-file" + complexify
	pathIsAFileAbsolute      = string(os.PathSeparator) + "is-a-file" + complexify
	pathIsADirRelative       = "is-a-dir" + complexify
	pathIsADirAbsolute       = string(os.PathSeparator) + "is-a-dir" + complexify
	pathIsAVolumeMount       = string(os.PathSeparator) + "is-a-volume-mount" + complexify

	srcFileName  = "test-file" + complexify
	tarballName  = "test-tar" + complexify
	cpFolderName = "nerdctl-cp-test"

	// Since nerdctl cp must NOT obey container wd, but instead resolve paths against the root, we set this
	// explicitly to ensure we do the right thing wrt that.
	containerCwd = "/nerdctl/cp/test"

	dirPerm  = 0o755
	filePerm = 0o644
)

var srcDirName = filepath.Join("three-levels-src-dir", "test-dir", "dir"+complexify)

type testgroup struct {
	description string // parent test description
	toContainer bool   // copying to, or from container

	// sourceSpec as specified by the user (without the container: part) - can be relative or absolute -
	// if sourceSpec points to a file, you must use srcFileName for filename
	sourceSpec    string
	sourceIsAFile bool        // whether the provided sourceSpec points to a file or a dir
	testCases     []testcases // testcases
}

type testcases struct {
	description     string        // textual description of what the test is doing
	destinationSpec string        // destination path as specified by the user (without the container: part) - can be relative or absolute
	expect          icmd.Expected // expectation

	// Optional
	catFile  string                                                        // path that we "cat" - defaults to destinationSpec if not specified
	setup    func(helpers test.Helpers, container string, destPath string) // additional test setup if needed
	tearDown func()                                                        // additional cleanup if needed
	volume   func(helpers test.Helpers, id string) (string, string, bool)  // volume creation function if needed (should return the volume name, mountPoint, readonly flag)
}

func TestCopyToContainer(t *testing.T) {
	testGroups := []*testgroup{
		{
			description:   "Copying to container, SRC_PATH is a file, absolute",
			sourceSpec:    filepath.Join(string(os.PathSeparator), srcDirName, srcFileName),
			sourceIsAFile: true,
			toContainer:   true,
			testCases: []testcases{
				{
					description:     "DEST_PATH does not exist, relative",
					destinationSpec: pathDoesNotExistRelative,
					expect: icmd.Expected{
						ExitCode: 0,
					},
				},
				{
					description:     "DEST_PATH does not exist, absolute",
					destinationSpec: pathDoesNotExistAbsolute,
					expect: icmd.Expected{
						ExitCode: 0,
					},
				},
				{
					description:     "DEST_PATH does not exist, relative, and ends with " + string(os.PathSeparator),
					destinationSpec: pathDoesNotExistRelative + string(os.PathSeparator),
					expect: icmd.Expected{
						ExitCode: 1,
						Err:      containerutil.ErrDestinationDirMustExist.Error(),
					},
				},
				{
					description:     "DEST_PATH does not exist, absolute, and ends with " + string(os.PathSeparator),
					destinationSpec: pathDoesNotExistAbsolute + string(os.PathSeparator),
					expect: icmd.Expected{
						ExitCode: 1,
						Err:      containerutil.ErrDestinationDirMustExist.Error(),
					},
				},

				{
					description:     "DEST_PATH is a file, relative",
					destinationSpec: pathIsAFileRelative,
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						helpers.Ensure("exec", container, "touch", destPath)
					},
				},
				{
					description:     "DEST_PATH is a file, absolute",
					destinationSpec: pathIsAFileAbsolute,
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						helpers.Ensure("exec", container, "touch", destPath)
					},
				},
				{
					description:     "DEST_PATH is a file, relative, ends with improper " + string(os.PathSeparator),
					destinationSpec: pathIsAFileRelative + string(os.PathSeparator),
					expect: icmd.Expected{
						ExitCode: 1,
						Err:      containerutil.ErrDestinationIsNotADir.Error(),
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						helpers.Ensure("exec", container, "touch", destPath)
					},
				},
				{
					description:     "DEST_PATH is a file, absolute, ends with improper " + string(os.PathSeparator),
					destinationSpec: pathIsAFileAbsolute + string(os.PathSeparator),
					expect: icmd.Expected{
						ExitCode: 1,
						// FIXME: it is unclear why the code path with absolute (this test) versus relative (just above)
						// yields a different error. Both should ideally be ErrCannotCopyDirToFile
						// This is probably happening somewhere in resolve.
						// This is not a deal killer, as both DO error with a reasonable explanation, but a bit
						// frustrating
						Err: containerutil.ErrDestinationIsNotADir.Error(),
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						helpers.Ensure("exec", container, "touch", destPath)
					},
				},
				{
					description:     "DEST_PATH is a directory, relative",
					destinationSpec: pathIsADirRelative,
					catFile:         filepath.Join(pathIsADirRelative, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						helpers.Ensure("exec", container, "mkdir", "-p", destPath)
					},
				},
				{
					description:     "DEST_PATH is a directory, absolute",
					destinationSpec: pathIsADirAbsolute,
					catFile:         filepath.Join(pathIsADirAbsolute, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						helpers.Ensure("exec", container, "mkdir", "-p", destPath)
					},
				},
				{
					description:     "DEST_PATH is a directory, relative, ends with " + string(os.PathSeparator),
					destinationSpec: pathIsADirRelative + string(os.PathSeparator),
					catFile:         filepath.Join(pathIsADirRelative, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						helpers.Ensure("exec", container, "mkdir", "-p", destPath)
					},
				},
				{
					description:     "DEST_PATH is a directory, absolute, ends with " + string(os.PathSeparator),
					destinationSpec: pathIsADirAbsolute + string(os.PathSeparator),
					catFile:         filepath.Join(pathIsADirAbsolute, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						helpers.Ensure("exec", container, "mkdir", "-p", destPath)
					},
				},
				{
					description:     "DEST_PATH is a volume mount-point",
					destinationSpec: pathIsAVolumeMount,
					catFile:         filepath.Join(pathIsAVolumeMount, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					// FIXME the way we handle volume is not right - too complicated for the test author
					volume: func(helpers test.Helpers, id string) (string, string, bool) {
						helpers.Ensure("volume", "create", id)
						return id, pathIsAVolumeMount, false
					},
				},
				{
					description:     "DEST_PATH is a read-only volume mount-point",
					destinationSpec: pathIsAVolumeMount,
					expect: icmd.Expected{
						ExitCode: 1,
						Err:      containerutil.ErrTargetIsReadOnly.Error(),
					},
					volume: func(helpers test.Helpers, id string) (string, string, bool) {
						helpers.Ensure("volume", "create", id)
						return id, pathIsAVolumeMount, true
					},
				},
			},
		},
		{
			description: "Copying to container, SRC_PATH is a directory",
			sourceSpec:  srcDirName,
			toContainer: true,
			testCases: []testcases{
				{
					description:     "DEST_PATH does not exist, relative",
					destinationSpec: pathDoesNotExistRelative,
					catFile:         filepath.Join(pathDoesNotExistRelative, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
				},
				{
					description:     "DEST_PATH does not exist, absolute",
					destinationSpec: pathDoesNotExistAbsolute,
					catFile:         filepath.Join(pathDoesNotExistAbsolute, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
				},
				{
					description:     "DEST_PATH does not exist, relative, and ends with " + string(os.PathSeparator),
					destinationSpec: pathDoesNotExistRelative + string(os.PathSeparator),
					catFile:         filepath.Join(pathDoesNotExistRelative, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
				},
				{
					description:     "DEST_PATH does not exist, absolute, and ends with " + string(os.PathSeparator),
					destinationSpec: pathDoesNotExistAbsolute + string(os.PathSeparator),
					catFile:         filepath.Join(pathDoesNotExistAbsolute, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
				},
				{
					description:     "DEST_PATH is a file, relative",
					destinationSpec: pathIsAFileRelative,
					expect: icmd.Expected{
						ExitCode: 1,
						Err:      containerutil.ErrCannotCopyDirToFile.Error(),
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						helpers.Ensure("exec", container, "touch", destPath)
					},
				},
				{
					description:     "DEST_PATH is a file, absolute",
					destinationSpec: pathIsAFileAbsolute,
					expect: icmd.Expected{
						ExitCode: 1,
						Err:      containerutil.ErrCannotCopyDirToFile.Error(),
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						helpers.Ensure("exec", container, "touch", destPath)
					},
				},
				{
					description:     "DEST_PATH is a file, relative, ends with improper " + string(os.PathSeparator),
					destinationSpec: pathIsAFileRelative + string(os.PathSeparator),
					expect: icmd.Expected{
						ExitCode: 1,
						Err:      containerutil.ErrDestinationIsNotADir.Error(),
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						helpers.Ensure("exec", container, "touch", destPath)
					},
				},
				{
					description:     "DEST_PATH is a file, absolute, ends with improper " + string(os.PathSeparator),
					destinationSpec: pathIsAFileAbsolute + string(os.PathSeparator),
					expect: icmd.Expected{
						ExitCode: 1,
						// FIXME: it is unclear why the code path with absolute (this test) versus relative (just above)
						// yields a different error. Both should ideally be ErrCannotCopyDirToFile
						// This is probably happening somewhere in resolve.
						// This is not a deal killer, as both DO error with a reasonable explanation, but a bit
						// frustrating
						Err: containerutil.ErrDestinationIsNotADir.Error(),
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						helpers.Ensure("exec", container, "touch", destPath)
					},
				},
				{
					description:     "DEST_PATH is a directory, relative",
					destinationSpec: pathIsADirRelative,
					catFile:         filepath.Join(pathIsADirRelative, filepath.Base(srcDirName), srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						helpers.Ensure("exec", container, "mkdir", "-p", destPath)
					},
				},
				{
					description:     "DEST_PATH is a directory, absolute",
					destinationSpec: pathIsADirAbsolute,
					catFile:         filepath.Join(pathIsADirAbsolute, filepath.Base(srcDirName), srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						helpers.Ensure("exec", container, "mkdir", "-p", destPath)
					},
				},
				{
					description:     "DEST_PATH is a directory, relative, ends with " + string(os.PathSeparator),
					destinationSpec: pathIsADirRelative + string(os.PathSeparator),
					catFile:         filepath.Join(pathIsADirRelative, filepath.Base(srcDirName), srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						helpers.Ensure("exec", container, "mkdir", "-p", destPath)
					},
				},
				{
					description:     "DEST_PATH is a directory, absolute, ends with " + string(os.PathSeparator),
					destinationSpec: pathIsADirAbsolute + string(os.PathSeparator),
					catFile:         filepath.Join(pathIsADirAbsolute, filepath.Base(srcDirName), srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						helpers.Ensure("exec", container, "mkdir", "-p", destPath)
					},
				},
			},
		},
		{
			description: "Copying to container, SRC_PATH is a directory ending with /.",
			sourceSpec:  srcDirName + string(os.PathSeparator) + ".",
			toContainer: true,
			testCases: []testcases{
				{
					description:     "DEST_PATH is a directory, relative",
					destinationSpec: pathIsADirRelative,
					catFile:         filepath.Join(pathIsADirRelative, srcFileName),
					setup: func(helpers test.Helpers, container string, destPath string) {
						helpers.Ensure("exec", container, "mkdir", "-p", destPath)
					},
				},
				{
					description:     "DEST_PATH is a directory, absolute",
					destinationSpec: pathIsADirAbsolute,
					catFile:         filepath.Join(pathIsADirAbsolute, srcFileName),
					setup: func(helpers test.Helpers, container string, destPath string) {
						helpers.Ensure("exec", container, "mkdir", "-p", destPath)
					},
				},
			},
		},
		{
			description:   "Copying to container, SRC_PATH is stdin",
			sourceSpec:    "-",
			sourceIsAFile: true,
			toContainer:   true,
			testCases: []testcases{
				{
					description:     "DEST_PATH is a directory, relative",
					destinationSpec: pathIsADirRelative,
					catFile:         filepath.Join(pathIsADirRelative, srcFileName),
					setup: func(helpers test.Helpers, container string, destPath string) {
						helpers.Ensure("exec", container, "mkdir", "-p", destPath)
					},
				},
				{
					description:     "DEST_PATH is a directory, absolute",
					destinationSpec: pathIsADirAbsolute,
					catFile:         filepath.Join(pathIsADirAbsolute, srcFileName),
					setup: func(helpers test.Helpers, container string, destPath string) {
						helpers.Ensure("exec", container, "mkdir", "-p", destPath)
					},
				},
				{
					description:     "DEST_PATH is stdout",
					destinationSpec: "-",
					expect: icmd.Expected{
						ExitCode: 1,
						Err:      "one of src or dest must be a container file specification",
					},
				},
				{
					description:     "DEST_PATH is a file",
					destinationSpec: pathIsAFileAbsolute,
					setup: func(helpers test.Helpers, container string, destPath string) {
						helpers.Ensure("exec", container, "touch", destPath)
					},
					expect: icmd.Expected{
						ExitCode: 1,
						Err:      containerutil.ErrCannotCopyDirToFile.Error(),
					},
				},
			},
		},
	}

	testCase := nerdtest.Setup()
	testCase.SubTests = cpBuildSubTests(testGroups)
	testCase.Run(t)
}

func TestCopyFromContainer(t *testing.T) {
	testGroups := []*testgroup{
		{
			description:   "Copying from container, SRC_PATH specifies a file",
			sourceSpec:    srcFileName,
			sourceIsAFile: true,
			testCases: []testcases{
				{
					description:     "DEST_PATH does not exist, relative",
					destinationSpec: pathDoesNotExistRelative,
					expect: icmd.Expected{
						ExitCode: 0,
					},
				},
				{
					description:     "DEST_PATH does not exist, absolute",
					destinationSpec: pathDoesNotExistAbsolute,
					expect: icmd.Expected{
						ExitCode: 0,
					},
				},
				{
					description:     "DEST_PATH does not exist, relative, and ends with a path separator",
					destinationSpec: pathDoesNotExistRelative + string(os.PathSeparator),
					expect: icmd.Expected{
						ExitCode: 1,
						Err:      containerutil.ErrDestinationDirMustExist.Error(),
					},
				},
				{
					description:     "DEST_PATH does not exist, absolute, and ends with a path separator",
					destinationSpec: pathDoesNotExistAbsolute + string(os.PathSeparator),
					expect: icmd.Expected{
						ExitCode: 1,
						Err:      containerutil.ErrDestinationDirMustExist.Error(),
					},
				},
				{
					description:     "DEST_PATH is a file, relative",
					destinationSpec: pathIsAFileRelative,
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						err := os.WriteFile(destPath, []byte(""), filePerm)
						assert.NilError(helpers.T(), err)
					},
				},
				{
					description:     "DEST_PATH is a file, absolute",
					destinationSpec: pathIsAFileAbsolute,
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						err := os.WriteFile(destPath, []byte(""), filePerm)
						assert.NilError(helpers.T(), err)
					},
				},
				{
					description:     "DEST_PATH is a file, relative, improperly ends with a separator",
					destinationSpec: pathIsAFileRelative + string(os.PathSeparator),
					expect: icmd.Expected{
						ExitCode: 1,
						Err:      containerutil.ErrDestinationIsNotADir.Error(),
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						err := os.WriteFile(destPath, []byte(""), filePerm)
						assert.NilError(helpers.T(), err)
					},
				},
				{
					description:     "DEST_PATH is a file, absolute, improperly ends with a separator",
					destinationSpec: pathIsAFileAbsolute + string(os.PathSeparator),
					expect: icmd.Expected{
						ExitCode: 1,
						Err:      containerutil.ErrDestinationIsNotADir.Error(),
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						err := os.WriteFile(destPath, []byte(""), filePerm)
						assert.NilError(helpers.T(), err)
					},
				},
				{
					description:     "DEST_PATH is a directory, relative",
					destinationSpec: pathIsADirRelative,
					catFile:         filepath.Join(pathIsADirRelative, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						err := os.MkdirAll(destPath, dirPerm)
						assert.NilError(helpers.T(), err)
					},
				},
				{
					description:     "DEST_PATH is a directory, absolute",
					destinationSpec: pathIsADirAbsolute,
					catFile:         filepath.Join(pathIsADirAbsolute, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						err := os.MkdirAll(destPath, dirPerm)
						assert.NilError(helpers.T(), err)
					},
				},
				{
					description:     "DEST_PATH is a directory, relative, ending with a path separator",
					destinationSpec: pathIsADirRelative + string(os.PathSeparator),
					catFile:         filepath.Join(pathIsADirRelative, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						err := os.MkdirAll(destPath, dirPerm)
						assert.NilError(helpers.T(), err)
					},
				},
				{
					description:     "DEST_PATH is a directory, absolute, ending with a path separator",
					destinationSpec: pathIsADirAbsolute + string(os.PathSeparator),
					catFile:         filepath.Join(pathIsADirAbsolute, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						err := os.MkdirAll(destPath, dirPerm)
						assert.NilError(helpers.T(), err)
					},
				},
				{
					description:     "DEST_PATH is stdout",
					destinationSpec: "-",
					// Extra dir to account for folder created from extracted tar file
					catFile: filepath.Join(pathIsADirAbsolute, filepath.Base(srcDirName), srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						err := os.MkdirAll(destPath, dirPerm)
						assert.NilError(helpers.T(), err)
					},
				},
			},
		},
		{
			description: "Copying from container, SRC_PATH specifies a dir",
			sourceSpec:  srcDirName,
			testCases: []testcases{
				{
					description:     "DEST_PATH does not exist, relative",
					destinationSpec: pathDoesNotExistRelative,
					catFile:         filepath.Join(pathDoesNotExistRelative, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
				},
				{
					description:     "DEST_PATH does not exist, absolute",
					destinationSpec: pathDoesNotExistAbsolute,
					catFile:         filepath.Join(pathDoesNotExistAbsolute, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
				},
				{
					description:     "DEST_PATH does not exist, relative, ends with path separator",
					destinationSpec: pathDoesNotExistRelative + string(os.PathSeparator),
					catFile:         filepath.Join(pathDoesNotExistRelative, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
				},
				{
					description:     "DEST_PATH does not exist, absolute, ends with path separator",
					destinationSpec: pathDoesNotExistAbsolute + string(os.PathSeparator),
					catFile:         filepath.Join(pathDoesNotExistAbsolute, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
				},
				{
					description:     "DEST_PATH is a file, relative",
					destinationSpec: pathIsAFileRelative,
					expect: icmd.Expected{
						ExitCode: 1,
						Err:      containerutil.ErrCannotCopyDirToFile.Error(),
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						err := os.WriteFile(destPath, []byte(""), filePerm)
						assert.NilError(helpers.T(), err)
					},
				},
				{
					description:     "DEST_PATH is a file, absolute",
					destinationSpec: pathIsAFileAbsolute,
					expect: icmd.Expected{
						ExitCode: 1,
						Err:      containerutil.ErrCannotCopyDirToFile.Error(),
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						err := os.WriteFile(destPath, []byte(""), filePerm)
						assert.NilError(helpers.T(), err)
					},
				},
				{
					description:     "DEST_PATH is a file, relative, improperly ends with path separator",
					destinationSpec: pathIsAFileRelative + string(os.PathSeparator),
					expect: icmd.Expected{
						ExitCode: 1,
						Err:      containerutil.ErrDestinationIsNotADir.Error(),
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						err := os.WriteFile(destPath, []byte(""), filePerm)
						assert.NilError(helpers.T(), err)
					},
				},
				{
					description:     "DEST_PATH is a file, absolute, improperly ends with path separator",
					destinationSpec: pathIsAFileAbsolute + string(os.PathSeparator),
					expect: icmd.Expected{
						ExitCode: 1,
						Err:      containerutil.ErrDestinationIsNotADir.Error(),
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						err := os.WriteFile(destPath, []byte(""), filePerm)
						assert.NilError(helpers.T(), err)
					},
				},
				{
					description:     "DEST_PATH is a directory, relative",
					destinationSpec: pathIsADirRelative,
					catFile:         filepath.Join(pathIsADirRelative, filepath.Base(srcDirName), srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						err := os.MkdirAll(destPath, dirPerm)
						assert.NilError(helpers.T(), err)
					},
				},
				{
					description:     "DEST_PATH is a directory, absolute",
					destinationSpec: pathIsADirAbsolute,
					catFile:         filepath.Join(pathIsADirAbsolute, filepath.Base(srcDirName), srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						err := os.MkdirAll(destPath, dirPerm)
						assert.NilError(helpers.T(), err)
					},
				},
				{
					description:     "DEST_PATH is a directory, relative, ends with path separator",
					destinationSpec: pathIsADirRelative + string(os.PathSeparator),
					catFile:         filepath.Join(pathIsADirRelative, filepath.Base(srcDirName), srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						err := os.MkdirAll(destPath, dirPerm)
						assert.NilError(helpers.T(), err)
					},
				},
				{
					description:     "DEST_PATH is a directory, absolute, ends with path separator",
					destinationSpec: pathIsADirAbsolute + string(os.PathSeparator),
					catFile:         filepath.Join(pathIsADirAbsolute, filepath.Base(srcDirName), srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						err := os.MkdirAll(destPath, dirPerm)
						assert.NilError(helpers.T(), err)
					},
				},
				{
					description:     "DEST_PATH is stdout",
					destinationSpec: "-",
					catFile:         filepath.Join(pathIsADirAbsolute, srcDirName, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						err := os.MkdirAll(destPath, dirPerm)
						assert.NilError(helpers.T(), err)
					},
				},
			},
		},

		{
			description: "SRC_PATH is a dir, with a trailing slash/dot",
			sourceSpec:  srcDirName + string(os.PathSeparator) + ".",
			testCases: []testcases{
				{
					description:     "DEST_PATH is a directory, relative",
					destinationSpec: pathIsADirRelative,
					catFile:         filepath.Join(pathIsADirRelative, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						err := os.MkdirAll(destPath, dirPerm)
						assert.NilError(helpers.T(), err)
					},
				},
				{
					description:     "DEST_PATH is a directory, absolute",
					destinationSpec: pathIsADirAbsolute,
					catFile:         filepath.Join(pathIsADirAbsolute, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						err := os.MkdirAll(destPath, dirPerm)
						assert.NilError(helpers.T(), err)
					},
				},
				{
					description:     "DEST_PATH is stdout",
					destinationSpec: "-",
					catFile:         filepath.Join(pathIsADirAbsolute, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(helpers test.Helpers, container string, destPath string) {
						err := os.MkdirAll(destPath, dirPerm)
						assert.NilError(helpers.T(), err)
					},
				},
			},
		},
	}

	testCase := nerdtest.Setup()
	testCase.SubTests = cpBuildSubTests(testGroups)
	testCase.Run(t)
}

func assertCatHelper(helpers test.Helpers, t tig.T, catPath string, fileContent []byte, container string, expectedUID int, containerIsStopped bool) {
	t.Log(fmt.Sprintf("catPath=%q", catPath))
	if container != "" && containerIsStopped {
		helpers.Ensure("start", container)
		defer func() { helpers.Ensure("stop", container) }()
	}

	if container == "" {
		got, err := os.ReadFile(catPath)
		assert.NilError(t, err, "Failed reading from file")
		assert.DeepEqual(t, fileContent, got)
		st, err := os.Stat(catPath)
		assert.NilError(t, err)
		stSys := st.Sys().(*syscall.Stat_t)
		expected := uint32(expectedUID)
		actual := stSys.Uid
		assert.DeepEqual(t, expected, actual)
	} else {
		content := helpers.Capture("exec", container, "sh", "-c", "--", fmt.Sprintf("ls -lA /; echo %q; cat %q", catPath, catPath))
		assert.Assert(t, strings.Contains(content, string(fileContent)))
		uid := helpers.Capture("exec", container, "stat", "-c", "%u", catPath)
		assert.Assert(t, uid == fmt.Sprintf("%d\n", expectedUID))
	}
}

func cpCreateFileOnHost(t tig.T, sourceFile string, sourceFileContent []byte, fromStdin bool) {
	if fromStdin {
		d := filepath.Dir(sourceFile)
		tarCpFolder := filepath.Join(d, cpFolderName)
		tarBinary, _, err := tarutil.FindTarBinary()
		assert.NilError(t, err)
		err = os.MkdirAll(tarCpFolder, dirPerm)
		assert.NilError(t, err)
		err = os.WriteFile(filepath.Join(tarCpFolder, srcFileName), sourceFileContent, filePerm)
		assert.NilError(t, err)
		err = exec.Command(tarBinary, "-cf", sourceFile, "-C", tarCpFolder, ".").Run()
		assert.NilError(t, err)
		err = os.RemoveAll(tarCpFolder)
		assert.NilError(t, err)
	} else {
		err := os.MkdirAll(filepath.Dir(sourceFile), dirPerm)
		assert.NilError(t, err)
		err = os.WriteFile(sourceFile, sourceFileContent, filePerm)
		assert.NilError(t, err)
	}
}

func cpBuildSubTests(testGroups []*testgroup) []*test.Case {
	var groupSubTests []*test.Case
	for _, tg := range testGroups {
		groupSubTests = append(groupSubTests, &test.Case{
			Description: tg.description,
			SubTests:    cpBuildCaseSubTests(tg),
		})
	}
	return groupSubTests
}

func cpBuildCaseSubTests(tg *testgroup) []*test.Case {
	groupSourceSpec := tg.sourceSpec
	groupSourceDir := groupSourceSpec
	fromStdin := false
	if tg.sourceSpec == "-" {
		groupSourceSpec = filepath.Join(srcDirName, tarballName)
		groupSourceDir = srcDirName
		fromStdin = true
	} else if tg.sourceIsAFile {
		groupSourceDir = filepath.Dir(groupSourceSpec)
	}
	copyToContainer := tg.toContainer
	var srcUID, destUID int
	if copyToContainer {
		srcUID = os.Geteuid()
		destUID = srcUID
	} else {
		srcUID = 42
		destUID = os.Geteuid()
	}
	var subTests []*test.Case
	for _, tc := range tg.testCases {
		subTests = append(subTests, cpSingleCaseSubTest(tc, copyToContainer, groupSourceSpec, groupSourceDir, fromStdin, srcUID, destUID))
	}
	return subTests
}

func cpSingleCaseSubTest(tc testcases, copyToContainer bool, groupSourceSpec, groupSourceDir string, fromStdin bool, srcUID, destUID int) *test.Case {
	return &test.Case{
		Description: tc.description,
		NoParallel:  true,
		Setup: func(data test.Data, helpers test.Helpers) {
			testID := data.Identifier()
			containerRunning := testID + "-r"
			containerStopped := testID + "-s"
			sourceFileContent := []byte(testID)
			tempDir := data.Temp().Dir("work")
			data.Labels().Set("containerRunning", containerRunning)
			data.Labels().Set("containerStopped", containerStopped)
			data.Labels().Set("sourceFileContent", string(sourceFileContent))
			data.Labels().Set("tempDir", tempDir)

			sourceSpec := groupSourceSpec
			catFile := tc.catFile
			destinationSpec := tc.destinationSpec
			toStdout := false
			if destinationSpec == "-" {
				toStdout = true
				destinationSpec = filepath.Dir(catFile)
			}
			if catFile == "" {
				catFile = destinationSpec
			}
			sourceFile := filepath.Join(groupSourceDir, srcFileName)
			if copyToContainer {
				if !filepath.IsAbs(catFile) {
					catFile = filepath.Join(string(os.PathSeparator), catFile)
				}
				if fromStdin {
					sourceFile = filepath.Join(tempDir, groupSourceDir, tarballName)
				} else {
					sourceFile = filepath.Join(tempDir, sourceFile)
					if filepath.IsAbs(sourceSpec) {
						sourceSpec = tempDir + sourceSpec
					}
				}
			} else {
				catFile = filepath.Join(tempDir, catFile)
				if filepath.IsAbs(destinationSpec) {
					destinationSpec = tempDir + destinationSpec
				}
			}
			data.Labels().Set("sourceSpec", sourceSpec)
			data.Labels().Set("catFile", catFile)
			data.Labels().Set("destinationSpec", destinationSpec)
			data.Labels().Set("sourceFile", sourceFile)
			if toStdout {
				data.Labels().Set("toStdout", "true")
			}

			args := []string{"run", "-d", "-w", containerCwd}
			if tc.volume != nil {
				vol, mount, ro := tc.volume(helpers, testID)
				volArg := fmt.Sprintf("%s:%s", vol, mount)
				if ro {
					volArg += ":ro"
				}
				args = append(args, "-v", volArg)
			}
			helpers.Ensure(append(args, "--name", containerRunning, testutil.CommonImage, "sleep", nerdtest.Infinity)...)
			helpers.Ensure(append(args, "--name", containerStopped, testutil.CommonImage, "sleep", nerdtest.Infinity)...)
			if copyToContainer {
				cpCreateFileOnHost(helpers.T(), sourceFile, sourceFileContent, fromStdin)
			} else {
				mkSrcScript := fmt.Sprintf("cd /; mkdir -p %q && echo -n %q >%q && chown %d %q", filepath.Dir(sourceFile), sourceFileContent, sourceFile, srcUID, sourceFile)
				helpers.Ensure("exec", containerRunning, "sh", "-euc", mkSrcScript)
				helpers.Ensure("exec", containerStopped, "sh", "-euc", mkSrcScript)
			}
			if tc.setup != nil {
				setupDest := strings.TrimSuffix(destinationSpec, string(os.PathSeparator))
				if !filepath.IsAbs(setupDest) {
					if copyToContainer {
						setupDest = filepath.Join(string(os.PathSeparator), setupDest)
					} else {
						setupDest = filepath.Join(tempDir, setupDest)
					}
				}
				tc.setup(helpers, containerRunning, setupDest)
				tc.setup(helpers, containerStopped, setupDest)
			}
			helpers.Ensure("stop", containerStopped)
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			testID := data.Identifier()
			helpers.Anyhow("rm", "-f", testID+"-r")
			helpers.Anyhow("rm", "-f", testID+"-s")
			if tc.volume != nil {
				helpers.Anyhow("volume", "rm", testID)
			}
			if tc.tearDown != nil {
				tc.tearDown()
			}
		},
		SubTests: []*test.Case{
			cpRunningSubTest(tc, copyToContainer, fromStdin, destUID),
			cpStoppedSubTest(tc, copyToContainer, fromStdin, destUID),
		},
	}
}

func cpRunningSubTest(tc testcases, copyToContainer bool, fromStdin bool, destUID int) *test.Case {
	return cpContainerSubTest("running container", tc, copyToContainer, fromStdin, destUID, false)
}

func cpStoppedSubTest(tc testcases, copyToContainer bool, fromStdin bool, destUID int) *test.Case {
	return cpContainerSubTest("stopped container", tc, copyToContainer, fromStdin, destUID, true)
}

func cpContainerSubTest(description string, tc testcases, copyToContainer bool, fromStdin bool, destUID int, stopped bool) *test.Case {
	return &test.Case{
		Description: description,
		NoParallel:  true,
		Setup: func(data test.Data, helpers test.Helpers) {
			if stopped && copyToContainer {
				tempDir := data.Labels().Get("tempDir")
				sourceFileContent := []byte(data.Labels().Get("sourceFileContent"))
				sourceFile := data.Labels().Get("sourceFile")
				err := os.RemoveAll(tempDir)
				assert.NilError(helpers.T(), err)
				err = os.MkdirAll(tempDir, dirPerm)
				assert.NilError(helpers.T(), err)
				cpCreateFileOnHost(helpers.T(), sourceFile, sourceFileContent, fromStdin)
			}
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			container := data.Labels().Get("containerRunning")
			if stopped {
				container = data.Labels().Get("containerStopped")
			}
			sourceSpec := data.Labels().Get("sourceSpec")
			sourceFile := data.Labels().Get("sourceFile")
			destinationSpec := data.Labels().Get("destinationSpec")
			tempDir := data.Labels().Get("tempDir")
			toStdout := data.Labels().Get("toStdout") == "true"
			if fromStdin && toStdout {
				return helpers.Command("cp", "-", "-")
			}
			if copyToContainer {
				if fromStdin {
					cmd := helpers.Command("cp", "-", container+":"+destinationSpec)
					cmd.WithFeeder(func() io.Reader {
						f, err := os.Open(sourceFile)
						assert.NilError(helpers.T(), err)
						return f
					})
					cmd.WithCwd(tempDir)
					return cmd
				}
				cmd := helpers.Command("cp", sourceSpec, container+":"+destinationSpec)
				cmd.WithCwd(tempDir)
				return cmd
			}
			cmd := helpers.Command("cp", container+":"+sourceSpec, destinationSpec)
			cmd.WithCwd(tempDir)
			return cmd
		},
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			toStdout := data.Labels().Get("toStdout") == "true"
			if stopped && rootlessutil.IsRootless() && !nerdtest.IsDocker() && !(fromStdin && toStdout) {
				return &test.Expected{ExitCode: 1, Errors: []error{containerutil.ErrRootlessCannotCp}}
			}
			exitCode := tc.expect.ExitCode
			expectErr := tc.expect.Err
			if nerdtest.IsDocker() {
				expectErr = ""
			}
			exp := &test.Expected{ExitCode: exitCode}
			if expectErr != "" {
				exp.Errors = []error{errors.New(expectErr)}
			}
			if exitCode == 0 {
				catFile := data.Labels().Get("catFile")
				if !copyToContainer && toStdout && data.Labels().Get("sourceSpec") == srcDirName {
					catFile = filepath.Join(filepath.Dir(catFile), filepath.Base(srcDirName), srcFileName)
				}
				sourceFileContent := []byte(data.Labels().Get("sourceFileContent"))
				container := ""
				if copyToContainer {
					container = data.Labels().Get("containerRunning")
					if stopped {
						container = data.Labels().Get("containerStopped")
					}
				}
				exp.Output = func(stdout string, t tig.T) {
					assertCatHelper(helpers, t, catFile, sourceFileContent, container, destUID, stopped)
				}
			}
			return exp
		},
	}
}
