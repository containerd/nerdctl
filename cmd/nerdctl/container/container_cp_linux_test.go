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
	"strings"
	"syscall"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"

	"github.com/containerd/nerdctl/v2/pkg/containerutil"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
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

	srcFileName = "test-file" + complexify

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
	catFile  string                                                       // path that we "cat" - defaults to destinationSpec if not specified
	setup    func(base *testutil.Base, container string, destPath string) // additional test setup if needed
	tearDown func()                                                       // additional cleanup if needed
	volume   func(base *testutil.Base, id string) (string, string, bool)  // volume creation function if needed (should return the volume name, mountPoint, readonly flag)
}

func TestCopyToContainer(t *testing.T) {
	t.Parallel()

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
					setup: func(base *testutil.Base, container string, destPath string) {
						base.Cmd("exec", container, "touch", destPath).AssertOK()
					},
				},
				{
					description:     "DEST_PATH is a file, absolute",
					destinationSpec: pathIsAFileAbsolute,
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						base.Cmd("exec", container, "touch", destPath).AssertOK()
					},
				},
				{
					description:     "DEST_PATH is a file, relative, ends with improper " + string(os.PathSeparator),
					destinationSpec: pathIsAFileRelative + string(os.PathSeparator),
					expect: icmd.Expected{
						ExitCode: 1,
						Err:      containerutil.ErrDestinationIsNotADir.Error(),
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						base.Cmd("exec", container, "touch", destPath).AssertOK()
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
					setup: func(base *testutil.Base, container string, destPath string) {
						base.Cmd("exec", container, "touch", destPath).AssertOK()
					},
				},
				{
					description:     "DEST_PATH is a directory, relative",
					destinationSpec: pathIsADirRelative,
					catFile:         filepath.Join(pathIsADirRelative, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						base.Cmd("exec", container, "mkdir", "-p", destPath).AssertOK()
					},
				},
				{
					description:     "DEST_PATH is a directory, absolute",
					destinationSpec: pathIsADirAbsolute,
					catFile:         filepath.Join(pathIsADirAbsolute, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						base.Cmd("exec", container, "mkdir", "-p", destPath).AssertOK()
					},
				},
				{
					description:     "DEST_PATH is a directory, relative, ends with " + string(os.PathSeparator),
					destinationSpec: pathIsADirRelative + string(os.PathSeparator),
					catFile:         filepath.Join(pathIsADirRelative, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						base.Cmd("exec", container, "mkdir", "-p", destPath).AssertOK()
					},
				},
				{
					description:     "DEST_PATH is a directory, absolute, ends with " + string(os.PathSeparator),
					destinationSpec: pathIsADirAbsolute + string(os.PathSeparator),
					catFile:         filepath.Join(pathIsADirAbsolute, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						base.Cmd("exec", container, "mkdir", "-p", destPath).AssertOK()
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
					volume: func(base *testutil.Base, id string) (string, string, bool) {
						base.Cmd("volume", "create", id).Run()
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
					volume: func(base *testutil.Base, id string) (string, string, bool) {
						base.Cmd("volume", "create", id).Run()
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
					setup: func(base *testutil.Base, container string, destPath string) {
						base.Cmd("exec", container, "touch", destPath).AssertOK()
					},
				},
				{
					description:     "DEST_PATH is a file, absolute",
					destinationSpec: pathIsAFileAbsolute,
					expect: icmd.Expected{
						ExitCode: 1,
						Err:      containerutil.ErrCannotCopyDirToFile.Error(),
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						base.Cmd("exec", container, "touch", destPath).AssertOK()
					},
				},
				{
					description:     "DEST_PATH is a file, relative, ends with improper " + string(os.PathSeparator),
					destinationSpec: pathIsAFileRelative + string(os.PathSeparator),
					expect: icmd.Expected{
						ExitCode: 1,
						Err:      containerutil.ErrDestinationIsNotADir.Error(),
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						base.Cmd("exec", container, "touch", destPath).AssertOK()
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
					setup: func(base *testutil.Base, container string, destPath string) {
						base.Cmd("exec", container, "touch", destPath).AssertOK()
					},
				},
				{
					description:     "DEST_PATH is a directory, relative",
					destinationSpec: pathIsADirRelative,
					catFile:         filepath.Join(pathIsADirRelative, filepath.Base(srcDirName), srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						base.Cmd("exec", container, "mkdir", "-p", destPath).AssertOK()
					},
				},
				{
					description:     "DEST_PATH is a directory, absolute",
					destinationSpec: pathIsADirAbsolute,
					catFile:         filepath.Join(pathIsADirAbsolute, filepath.Base(srcDirName), srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						base.Cmd("exec", container, "mkdir", "-p", destPath).AssertOK()
					},
				},
				{
					description:     "DEST_PATH is a directory, relative, ends with " + string(os.PathSeparator),
					destinationSpec: pathIsADirRelative + string(os.PathSeparator),
					catFile:         filepath.Join(pathIsADirRelative, filepath.Base(srcDirName), srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						base.Cmd("exec", container, "mkdir", "-p", destPath).AssertOK()
					},
				},
				{
					description:     "DEST_PATH is a directory, absolute, ends with " + string(os.PathSeparator),
					destinationSpec: pathIsADirAbsolute + string(os.PathSeparator),
					catFile:         filepath.Join(pathIsADirAbsolute, filepath.Base(srcDirName), srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						base.Cmd("exec", container, "mkdir", "-p", destPath).AssertOK()
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
					setup: func(base *testutil.Base, container string, destPath string) {
						base.Cmd("exec", container, "mkdir", "-p", destPath).AssertOK()
					},
				},
				{
					description:     "DEST_PATH is a directory, absolute",
					destinationSpec: pathIsADirAbsolute,
					catFile:         filepath.Join(pathIsADirAbsolute, srcFileName),
					setup: func(base *testutil.Base, container string, destPath string) {
						base.Cmd("exec", container, "mkdir", "-p", destPath).AssertOK()
					},
				},
			},
		},
	}

	for _, tg := range testGroups {
		cpTestHelper(t, tg)
	}
}

func TestCopyFromContainer(t *testing.T) {
	t.Parallel()

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
					setup: func(base *testutil.Base, container string, destPath string) {
						err := os.WriteFile(destPath, []byte(""), filePerm)
						assert.NilError(t, err)
					},
				},
				{
					description:     "DEST_PATH is a file, absolute",
					destinationSpec: pathIsAFileAbsolute,
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						err := os.WriteFile(destPath, []byte(""), filePerm)
						assert.NilError(t, err)
					},
				},
				{
					description:     "DEST_PATH is a file, relative, improperly ends with a separator",
					destinationSpec: pathIsAFileRelative + string(os.PathSeparator),
					expect: icmd.Expected{
						ExitCode: 1,
						Err:      containerutil.ErrDestinationIsNotADir.Error(),
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						err := os.WriteFile(destPath, []byte(""), filePerm)
						assert.NilError(t, err)
					},
				},
				{
					description:     "DEST_PATH is a file, absolute, improperly ends with a separator",
					destinationSpec: pathIsAFileAbsolute + string(os.PathSeparator),
					expect: icmd.Expected{
						ExitCode: 1,
						Err:      containerutil.ErrDestinationIsNotADir.Error(),
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						err := os.WriteFile(destPath, []byte(""), filePerm)
						assert.NilError(t, err)
					},
				},
				{
					description:     "DEST_PATH is a directory, relative",
					destinationSpec: pathIsADirRelative,
					catFile:         filepath.Join(pathIsADirRelative, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						err := os.MkdirAll(destPath, dirPerm)
						assert.NilError(t, err)
					},
				},
				{
					description:     "DEST_PATH is a directory, absolute",
					destinationSpec: pathIsADirAbsolute,
					catFile:         filepath.Join(pathIsADirAbsolute, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						err := os.MkdirAll(destPath, dirPerm)
						assert.NilError(t, err)
					},
				},
				{
					description:     "DEST_PATH is a directory, relative, ending with a path separator",
					destinationSpec: pathIsADirRelative + string(os.PathSeparator),
					catFile:         filepath.Join(pathIsADirRelative, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						err := os.MkdirAll(destPath, dirPerm)
						assert.NilError(t, err)
					},
				},
				{
					description:     "DEST_PATH is a directory, absolute, ending with a path separator",
					destinationSpec: pathIsADirAbsolute + string(os.PathSeparator),
					catFile:         filepath.Join(pathIsADirAbsolute, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						err := os.MkdirAll(destPath, dirPerm)
						assert.NilError(t, err)
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
					setup: func(base *testutil.Base, container string, destPath string) {
						err := os.MkdirAll(filepath.Dir(destPath), dirPerm)
						assert.NilError(t, err)
						err = os.WriteFile(destPath, []byte(""), filePerm)
						assert.NilError(t, err)
					},
				},
				{
					description:     "DEST_PATH is a file, absolute",
					destinationSpec: pathIsAFileAbsolute,
					expect: icmd.Expected{
						ExitCode: 1,
						Err:      containerutil.ErrCannotCopyDirToFile.Error(),
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						err := os.MkdirAll(filepath.Dir(destPath), dirPerm)
						assert.NilError(t, err)
						err = os.WriteFile(destPath, []byte(""), filePerm)
						assert.NilError(t, err)
					},
				},
				{
					description:     "DEST_PATH is a file, relative, improperly ends with path separator",
					destinationSpec: pathIsAFileRelative + string(os.PathSeparator),
					expect: icmd.Expected{
						ExitCode: 1,
						Err:      containerutil.ErrDestinationIsNotADir.Error(),
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						err := os.MkdirAll(filepath.Dir(destPath), dirPerm)
						assert.NilError(t, err)
						err = os.WriteFile(destPath, []byte(""), filePerm)
						assert.NilError(t, err)
					},
				},
				{
					description:     "DEST_PATH is a file, absolute, improperly ends with path separator",
					destinationSpec: pathIsAFileAbsolute + string(os.PathSeparator),
					expect: icmd.Expected{
						ExitCode: 1,
						Err:      containerutil.ErrDestinationIsNotADir.Error(),
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						err := os.MkdirAll(filepath.Dir(destPath), dirPerm)
						assert.NilError(t, err)
						err = os.WriteFile(destPath, []byte(""), filePerm)
						assert.NilError(t, err)
					},
				},
				{
					description:     "DEST_PATH is a directory, relative",
					destinationSpec: pathIsADirRelative,
					catFile:         filepath.Join(pathIsADirRelative, filepath.Base(srcDirName), srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						err := os.MkdirAll(destPath, dirPerm)
						assert.NilError(t, err)
					},
				},
				{
					description:     "DEST_PATH is a directory, absolute",
					destinationSpec: pathIsADirAbsolute,
					catFile:         filepath.Join(pathIsADirAbsolute, filepath.Base(srcDirName), srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						err := os.MkdirAll(destPath, dirPerm)
						assert.NilError(t, err)
					},
				},
				{
					description:     "DEST_PATH is a directory, relative, ends with path separator",
					destinationSpec: pathIsADirRelative + string(os.PathSeparator),
					catFile:         filepath.Join(pathIsADirRelative, filepath.Base(srcDirName), srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						err := os.MkdirAll(destPath, dirPerm)
						assert.NilError(t, err)
					},
				},
				{
					description:     "DEST_PATH is a directory, absolute, ends with path separator",
					destinationSpec: pathIsADirAbsolute + string(os.PathSeparator),
					catFile:         filepath.Join(pathIsADirAbsolute, filepath.Base(srcDirName), srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						err := os.MkdirAll(destPath, dirPerm)
						assert.NilError(t, err)
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
					setup: func(base *testutil.Base, container string, destPath string) {
						err := os.MkdirAll(destPath, dirPerm)
						assert.NilError(t, err)
					},
				},
				{
					description:     "DEST_PATH is a directory, absolute",
					destinationSpec: pathIsADirAbsolute,
					catFile:         filepath.Join(pathIsADirAbsolute, srcFileName),
					expect: icmd.Expected{
						ExitCode: 0,
					},
					setup: func(base *testutil.Base, container string, destPath string) {
						err := os.MkdirAll(destPath, dirPerm)
						assert.NilError(t, err)
					},
				},
			},
		},
	}

	for _, tg := range testGroups {
		cpTestHelper(t, tg)
	}
}

func assertCatHelper(base *testutil.Base, catPath string, fileContent []byte, container string, expectedUID int, containerIsStopped bool) {
	base.T.Logf("catPath=%q", catPath)
	if container != "" && containerIsStopped {
		base.Cmd("start", container).AssertOK()
		defer base.Cmd("stop", container).AssertOK()
	}

	if container == "" {
		got, err := os.ReadFile(catPath)
		assert.NilError(base.T, err, "Failed reading from file")
		assert.DeepEqual(base.T, fileContent, got)
		st, err := os.Stat(catPath)
		assert.NilError(base.T, err)
		stSys := st.Sys().(*syscall.Stat_t)
		expected := uint32(expectedUID)
		actual := stSys.Uid
		assert.DeepEqual(base.T, expected, actual)
	} else {
		base.Cmd("exec", container, "sh", "-c", "--", fmt.Sprintf("ls -lA /; echo %q; cat %q", catPath, catPath)).AssertOutContains(string(fileContent))
		base.Cmd("exec", container, "stat", "-c", "%u", catPath).AssertOutExactly(fmt.Sprintf("%d\n", expectedUID))
	}
}

func cpTestHelper(t *testing.T, tg *testgroup) {
	// Get the source path
	groupSourceSpec := tg.sourceSpec
	groupSourceDir := groupSourceSpec
	if tg.sourceIsAFile {
		groupSourceDir = filepath.Dir(groupSourceSpec)
	}

	// Copy direction
	copyToContainer := tg.toContainer
	// Description
	description := tg.description
	// Test cases
	testCases := tg.testCases

	// Compute UIDs dependent on cp direction
	var srcUID, destUID int
	if copyToContainer {
		srcUID = os.Geteuid()
		destUID = srcUID
	} else {
		srcUID = 42
		destUID = os.Geteuid()
	}

	t.Run(description, func(t *testing.T) {
		t.Parallel()

		for _, tc := range testCases {
			testCase := tc

			t.Run(testCase.description, func(t *testing.T) {
				t.Parallel()

				// Compute test-specific values
				testID := testutil.Identifier(t)
				containerRunning := testID + "-r"
				containerStopped := testID + "-s"
				sourceFileContent := []byte(testID)
				tempDir := t.TempDir()

				base := testutil.NewBase(t)
				// Change working directory for commands to execute to the newly created temp directory on the host
				// Note that ChDir won't do in a parallel context - and that setup func on the host below
				// has to deal with that problem separately by making sure relative paths are resolved against temp
				base.Dir = tempDir

				// Prepare the specs and derived variables
				sourceSpec := groupSourceSpec
				destinationSpec := testCase.destinationSpec

				// If the test case does not specify a catFile, start with the destination spec
				catFile := testCase.catFile
				if catFile == "" {
					catFile = destinationSpec
				}

				sourceFile := filepath.Join(groupSourceDir, srcFileName)
				if copyToContainer {
					// Use an absolute path for evaluation
					if !filepath.IsAbs(catFile) {
						catFile = filepath.Join(string(os.PathSeparator), catFile)
					}
					// If the sourceFile is still relative, make it absolute to the temp
					sourceFile = filepath.Join(tempDir, sourceFile)
					// If the spec path for source on the host was absolute, make sure we put that under tempDir
					if filepath.IsAbs(sourceSpec) {
						sourceSpec = tempDir + sourceSpec
					}
				} else {
					// If we are copying to host, we need to make sure we have an absolute path to cat, relative to temp,
					// whether it is relative, or "absolute"
					catFile = filepath.Join(tempDir, catFile)
					// If the spec for destination on the host was absolute, make sure we put that under tempDir
					if filepath.IsAbs(destinationSpec) {
						destinationSpec = tempDir + destinationSpec
					}
				}

				// Teardown: clean-up containers and optional volume
				tearDown := func() {
					base.Cmd("rm", "-f", containerRunning).Run()
					base.Cmd("rm", "-f", containerStopped).Run()
					if testCase.volume != nil {
						volID, _, _ := testCase.volume(base, testID)
						base.Cmd("volume", "rm", volID).Run()
					}
				}

				createFileOnHost := func() {
					// Create file on the host
					err := os.MkdirAll(filepath.Dir(sourceFile), dirPerm)
					assert.NilError(t, err)
					err = os.WriteFile(sourceFile, sourceFileContent, filePerm)
					assert.NilError(t, err)
				}

				// Setup: create volume, containers, create the source file
				setup := func() {
					args := []string{"run", "-d", "-w", containerCwd}
					if testCase.volume != nil {
						vol, mount, ro := testCase.volume(base, testID)
						volArg := fmt.Sprintf("%s:%s", vol, mount)
						if ro {
							volArg += ":ro"
						}
						args = append(args, "-v", volArg)
					}
					base.Cmd(append(args, "--name", containerRunning, testutil.CommonImage, "sleep", "Inf")...).AssertOK()
					base.Cmd(append(args, "--name", containerStopped, testutil.CommonImage, "sleep", "Inf")...).AssertOK()

					if copyToContainer {
						createFileOnHost()
					} else {
						// Create file content in the container
						// Note: cd /, otherwise we end-up in the container cwd, which is NOT obeyed by cp
						mkSrcScript := fmt.Sprintf("cd /; mkdir -p %q && echo -n %q >%q && chown %d %q", filepath.Dir(sourceFile), sourceFileContent, sourceFile, srcUID, sourceFile)
						base.Cmd("exec", containerRunning, "sh", "-euc", mkSrcScript).AssertOK()
						base.Cmd("exec", containerStopped, "sh", "-euc", mkSrcScript).AssertOK()
					}

					// If we have optional setup, run that now
					if testCase.setup != nil {
						// Some specs may come with a trailing slash (proper or improper)
						// Setup should still work in all cases (including if its a file), and get through to the actual test
						setupDest := destinationSpec
						setupDest = strings.TrimSuffix(setupDest, string(os.PathSeparator))
						if !filepath.IsAbs(setupDest) {
							if copyToContainer {
								setupDest = filepath.Join(string(os.PathSeparator), setupDest)
							} else {
								setupDest = filepath.Join(tempDir, setupDest)
							}
						}
						testCase.setup(base, containerRunning, setupDest)
						testCase.setup(base, containerStopped, setupDest)
					}

					// Stop the "stopped" container
					base.Cmd("stop", containerStopped).AssertOK()
				}

				tearDown()
				t.Cleanup(tearDown)
				// If we have custom teardown, do that
				if testCase.tearDown != nil {
					testCase.tearDown()
					t.Cleanup(testCase.tearDown)
				}

				// Do the setup
				setup()

				// If Docker, removes the err part of expectation
				if nerdtest.IsDocker() {
					testCase.expect.Err = ""
				}

				// Build the final src and dest specifiers, including `containerXYZ:`
				container := ""
				if copyToContainer {
					container = containerRunning
					base.Cmd("cp", sourceSpec, containerRunning+":"+destinationSpec).Assert(testCase.expect)
				} else {
					base.Cmd("cp", containerRunning+":"+sourceSpec, destinationSpec).Assert(testCase.expect)
				}

				// Run the actual test for the running container
				// If we expect the op to be a success, also check the destination file
				if testCase.expect.ExitCode == 0 {
					assertCatHelper(base, catFile, sourceFileContent, container, destUID, false)
				}

				// When copying container > host, we get shadowing from the previous container, possibly hiding failures
				// Solution: clear-up the tempDir
				if copyToContainer {
					err := os.RemoveAll(tempDir)
					assert.NilError(t, err)
					err = os.MkdirAll(tempDir, dirPerm)
					assert.NilError(t, err)
					createFileOnHost()
					defer os.RemoveAll(tempDir)
				}

				// ... and for the stopped container
				container = ""
				var cmd *testutil.Cmd
				if copyToContainer {
					container = containerStopped
					cmd = base.Cmd("cp", sourceSpec, containerStopped+":"+destinationSpec)
				} else {
					cmd = base.Cmd("cp", containerStopped+":"+sourceSpec, destinationSpec)
				}

				if rootlessutil.IsRootless() && !nerdtest.IsDocker() {
					cmd.Assert(
						icmd.Expected{
							ExitCode: 1,
							Err:      containerutil.ErrRootlessCannotCp.Error(),
						})
					return
				}

				cmd.Assert(testCase.expect)
				if testCase.expect.ExitCode == 0 {
					assertCatHelper(base, catFile, sourceFileContent, container, destUID, true)
				}
			})
		}
	})
}
