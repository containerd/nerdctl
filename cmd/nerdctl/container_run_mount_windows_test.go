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
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"gotest.tools/v3/assert"
)

func TestRunMountVolume(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)
	rwDir, err := os.MkdirTemp(t.TempDir(), "rw")
	if err != nil {
		t.Fatal(err)
	}
	roDir, err := os.MkdirTemp(t.TempDir(), "ro")
	if err != nil {
		t.Fatal(err)
	}
	rwVolName := tID + "-rw"
	roVolName := tID + "-ro"
	for _, v := range []string{rwVolName, roVolName} {
		defer base.Cmd("volume", "rm", "-f", v).Run()
		base.Cmd("volume", "create", v).AssertOK()
	}

	containerName := tID
	defer base.Cmd("rm", "-f", containerName).AssertOK()
	base.Cmd("run",
		"-d",
		"--name", containerName,
		"-v", fmt.Sprintf("%s:C:/mnt1", rwDir),
		"-v", fmt.Sprintf("%s:C:/mnt2:ro", roDir),
		"-v", fmt.Sprintf("%s:C:/mnt3", rwVolName),
		"-v", fmt.Sprintf("%s:C:/mnt4:ro", roVolName),
		testutil.CommonImage,
		"ping localhost -t",
	).AssertOK()

	base.Cmd("exec", containerName, "cmd", "/c", "echo -n str1 > C:/mnt1/file1").AssertOK()
	base.Cmd("exec", containerName, "cmd", "/c", "echo -n str2 > C:/mnt2/file2").AssertFail()
	base.Cmd("exec", containerName, "cmd", "/c", "echo -n str3 > C:/mnt3/file3").AssertOK()
	base.Cmd("exec", containerName, "cmd", "/c", "echo -n str4 > C:/mnt4/file4").AssertFail()
	base.Cmd("rm", "-f", containerName).AssertOK()

	base.Cmd("run",
		"--rm",
		"-v", fmt.Sprintf("%s:C:/mnt1", rwDir),
		"-v", fmt.Sprintf("%s:C:/mnt3", rwVolName),
		testutil.CommonImage,
		"cat", "C:/mnt1/file1", "C:/mnt3/file3",
	).AssertOutContainsAll("str1", "str3")
	base.Cmd("run",
		"--rm",
		"-v", fmt.Sprintf("%s:C:/mnt3/mnt1", rwDir),
		"-v", fmt.Sprintf("%s:C:/mnt3", rwVolName),
		testutil.CommonImage,
		"cat", "C:/mnt3/mnt1/file1", "C:/mnt3/file3",
	).AssertOutContainsAll("str1", "str3")
}

func TestRunMountVolumeInspect(t *testing.T) {
	base := testutil.NewBase(t)
	testContainer := testutil.Identifier(t)
	testVolume := testutil.Identifier(t)

	defer base.Cmd("volume", "rm", "-f", testVolume).Run()
	base.Cmd("volume", "create", testVolume).AssertOK()
	inspectVolume := base.InspectVolume(testVolume)
	namedVolumeSource := inspectVolume.Mountpoint

	base.Cmd(
		"run", "-d", "--name", testContainer,
		"-v", "C:/mnt1",
		"-v", "C:/mnt2:C:/mnt2",
		"-v", "\\\\.\\pipe\\containerd-containerd:\\\\.\\pipe\\containerd-containerd",
		"-v", fmt.Sprintf("%s:C:/mnt3", testVolume),
		testutil.CommonImage,
	).AssertOK()

	inspect := base.InspectContainer(testContainer)
	// convert array to map to get by key of Destination
	actual := make(map[string]dockercompat.MountPoint)
	for i := range inspect.Mounts {
		actual[inspect.Mounts[i].Destination] = inspect.Mounts[i]
	}

	expected := []struct {
		dest       string
		mountPoint dockercompat.MountPoint
	}{
		// anonymous volume
		{
			dest: "C:\\mnt1",
			mountPoint: dockercompat.MountPoint{
				Type:        "volume",
				Source:      "", // source of anonymous volume is a generated path, so here will not check it.
				Destination: "C:\\mnt1",
			},
		},

		// bind
		{
			dest: "C:\\mnt2",
			mountPoint: dockercompat.MountPoint{
				Type:        "bind",
				Source:      "C:\\mnt2",
				Destination: "C:\\mnt2",
			},
		},

		// named pipe
		{
			dest: "\\\\.\\pipe\\containerd-containerd",
			mountPoint: dockercompat.MountPoint{
				Type:        "npipe",
				Source:      "\\\\.\\pipe\\containerd-containerd",
				Destination: "\\\\.\\pipe\\containerd-containerd",
			},
		},

		// named volume
		{
			dest: "C:\\mnt3",
			mountPoint: dockercompat.MountPoint{
				Type:        "volume",
				Name:        testVolume,
				Source:      namedVolumeSource,
				Destination: "C:\\mnt3",
			},
		},
	}

	for i := range expected {
		testCase := expected[i]
		t.Logf("test volume[dest=%q]", testCase.dest)

		mountPoint, ok := actual[testCase.dest]
		assert.Assert(base.T, ok)

		assert.Equal(base.T, testCase.mountPoint.Type, mountPoint.Type)
		assert.Equal(base.T, testCase.mountPoint.Destination, mountPoint.Destination)

		if testCase.mountPoint.Source == "" {
			// for anonymous volumes, we want to make sure that the source is not the same as the destination
			assert.Assert(base.T, mountPoint.Source != testCase.mountPoint.Destination)
		} else {
			assert.Equal(base.T, testCase.mountPoint.Source, mountPoint.Source)
		}

		if testCase.mountPoint.Name != "" {
			assert.Equal(base.T, testCase.mountPoint.Name, mountPoint.Name)
		}
	}
}

func TestRunMountAnonymousVolume(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	base.Cmd("run", "--rm", "-v", "TestVolume:C:/mnt", testutil.CommonImage).AssertOK()

	// For docker-campatibility, Unrecognised volume spec: invalid volume specification: 'TestVolume'
	base.Cmd("run", "--rm", "-v", "TestVolume", testutil.CommonImage).AssertFail()

	// Destination must be an absolute path not named volume
	base.Cmd("run", "--rm", "-v", "TestVolume2:TestVolumes", testutil.CommonImage).AssertFail()
}

func TestRunMountRelativePath(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	base.Cmd("run", "--rm", "-v", "./mnt:C:/mnt1", testutil.CommonImage, "cmd").AssertOK()

	// Destination cannot be a relative path
	base.Cmd("run", "--rm", "-v", "./mnt", testutil.CommonImage).AssertFail()
	base.Cmd("run", "--rm", "-v", "./mnt:./mnt1", testutil.CommonImage, "cmd").AssertFail()
}

func TestRunMountNamedPipeVolume(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	base.Cmd("run", "--rm", "-v", `\\.\pipe\containerd-containerd`, testutil.CommonImage).AssertFail()
}

func TestRunMountVolumeSpec(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	base.Cmd("run", "--rm", "-v", `InvalidPathC:\TestVolume:C:\Mount`, testutil.CommonImage).AssertFail()
	base.Cmd("run", "--rm", "-v", `C:\TestVolume:C:\Mount:ro,rw:boot`, testutil.CommonImage).AssertFail()

	// If -v is an empty string, it will be ignored
	base.Cmd("run", "--rm", "-v", "", testutil.CommonImage).AssertOK()
}
