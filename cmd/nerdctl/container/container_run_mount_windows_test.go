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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestRunMountVolume(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		rwDir := data.Temp().Dir("rw")
		roDir := data.Temp().Dir("ro")
		rwVolName := data.Identifier("rw")
		roVolName := data.Identifier("ro")

		helpers.Ensure("volume", "create", rwVolName)
		helpers.Ensure("volume", "create", roVolName)

		helpers.Ensure("run",
			"-d",
			"--name", data.Identifier(),
			"-v", fmt.Sprintf("%s:C:/mnt1", rwDir),
			"-v", fmt.Sprintf("%s:C:/mnt2:ro", roDir),
			"-v", fmt.Sprintf("%s:C:/mnt3", rwVolName),
			"-v", fmt.Sprintf("%s:C:/mnt4:ro", roVolName),
			testutil.CommonImage,
			"ping localhost -t",
		)

		// Verify rw mounts are writable
		helpers.Ensure("exec", data.Identifier(), "cmd", "/c", "echo -n str1 > C:/mnt1/file1")
		helpers.Ensure("exec", data.Identifier(), "cmd", "/c", "echo -n str3 > C:/mnt3/file3")
		// Verify ro mounts are NOT writable
		helpers.Fail("exec", data.Identifier(), "cmd", "/c", "echo -n str2 > C:/mnt2/file2")
		helpers.Fail("exec", data.Identifier(), "cmd", "/c", "echo -n str4 > C:/mnt4/file4")

		helpers.Ensure("rm", "-f", data.Identifier())

		data.Labels().Set("rwDir", rwDir)
		data.Labels().Set("rwVolName", rwVolName)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "data persists across container removal",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run",
					"--rm",
					"-v", fmt.Sprintf("%s:C:/mnt1", data.Labels().Get("rwDir")),
					"-v", fmt.Sprintf("%s:C:/mnt3", data.Labels().Get("rwVolName")),
					testutil.CommonImage,
					"cat", "C:/mnt1/file1", "C:/mnt3/file3",
				)
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("str1", "str3")),
		},
		{
			Description: "nested mount ordering",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run",
					"--rm",
					"-v", fmt.Sprintf("%s:C:/mnt3/mnt1", data.Labels().Get("rwDir")),
					"-v", fmt.Sprintf("%s:C:/mnt3", data.Labels().Get("rwVolName")),
					testutil.CommonImage,
					"cat", "C:/mnt3/mnt1/file1", "C:/mnt3/file3",
				)
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("str1", "str3")),
		},
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
		helpers.Anyhow("volume", "rm", "-f", data.Identifier("rw"))
		helpers.Anyhow("volume", "rm", "-f", data.Identifier("ro"))
	}

	testCase.Run(t)
}

func TestRunMountVolumeInspect(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		testVolume := data.Identifier("vol")

		helpers.Ensure("volume", "create", testVolume)
		inspectVolume := nerdtest.InspectVolume(helpers, testVolume)
		data.Labels().Set("namedVolumeSource", inspectVolume.Mountpoint)
		data.Labels().Set("testVolume", testVolume)

		helpers.Ensure(
			"run", "-d", "--name", data.Identifier(),
			"-v", "C:/mnt1",
			"-v", "C:/mnt2:C:/mnt2",
			"-v", "\\\\.\\pipe\\containerd-containerd:\\\\.\\pipe\\containerd-containerd",
			"-v", fmt.Sprintf("%s:C:/mnt3", testVolume),
			testutil.CommonImage,
		)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
		helpers.Anyhow("volume", "rm", "-f", data.Identifier("vol"))
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("inspect", data.Identifier())
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, t tig.T) {
				var dc []dockercompat.Container

				err := json.Unmarshal([]byte(stdout), &dc)
				assert.NilError(t, err)
				assert.Equal(t, 1, len(dc))

				inspect := dc[0]
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
							Source:      "",
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
							Name:        data.Labels().Get("testVolume"),
							Source:      data.Labels().Get("namedVolumeSource"),
							Destination: "C:\\mnt3",
						},
					},
				}

				for i := range expected {
					tc := expected[i]

					mountPoint, ok := actual[tc.dest]
					assert.Assert(t, ok, "mount point not found for dest=%q", tc.dest)

					assert.Equal(t, tc.mountPoint.Type, mountPoint.Type)
					assert.Equal(t, tc.mountPoint.Destination, mountPoint.Destination)

					if tc.mountPoint.Source == "" {
						// for anonymous volumes, we want to make sure that the source is not the same as the destination
						assert.Assert(t, mountPoint.Source != tc.mountPoint.Destination)
					} else {
						assert.Equal(t, tc.mountPoint.Source, mountPoint.Source)
					}

					if tc.mountPoint.Name != "" {
						assert.Equal(t, tc.mountPoint.Name, mountPoint.Name)
					}
				}
			},
		}
	}

	testCase.Run(t)
}

func TestRunMountAnonymousVolume(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.SubTests = []*test.Case{
		{
			Description: "named volume with mount path",
			Command:     test.Command("run", "--rm", "-v", "TestVolume:C:/mnt", testutil.CommonImage),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
		{
			// For docker-compatibility, Unrecognised volume spec: invalid volume specification: 'TestVolume'
			Description: "volume name only fails",
			Command:     test.Command("run", "--rm", "-v", "TestVolume", testutil.CommonImage),
			Expected:    test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			Description: "non-absolute destination fails",
			Command:     test.Command("run", "--rm", "-v", "TestVolume2:TestVolumes", testutil.CommonImage),
			Expected:    test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
	}

	testCase.Run(t)
}

func TestRunMountRelativePath(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.SubTests = []*test.Case{
		{
			Description: "relative source with absolute destination",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("run", "--rm", "-v", "./mnt:C:/mnt1", testutil.CommonImage, "cmd")
				cmd.WithCwd(data.Temp().Dir())
				return cmd
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
		{
			// Destination cannot be a relative path
			Description: "relative source only fails",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("run", "--rm", "-v", "./mnt", testutil.CommonImage)
				cmd.WithCwd(data.Temp().Dir())
				return cmd
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			Description: "relative source and relative destination fails",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("run", "--rm", "-v", "./mnt:./mnt1", testutil.CommonImage, "cmd")
				cmd.WithCwd(data.Temp().Dir())
				return cmd
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
	}

	testCase.Run(t)
}

func TestRunMountNamedPipeVolume(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Command = test.Command("run", "--rm", "-v", `\\.\pipe\containerd-containerd`, testutil.CommonImage)
	testCase.Expected = test.Expects(expect.ExitCodeGenericFail, nil, nil)

	testCase.Run(t)
}

func TestRunMountVolumeSpec(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.SubTests = []*test.Case{
		{
			Description: "invalid source path",
			Command:     test.Command("run", "--rm", "-v", `InvalidPathC:\TestVolume:C:\Mount`, testutil.CommonImage),
			Expected:    test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			Description: "invalid mount options",
			Command:     test.Command("run", "--rm", "-v", `C:\TestVolume:C:\Mount:ro,rw:boot`, testutil.CommonImage),
			Expected:    test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			// If -v is an empty string, it will be ignored
			Description: "empty volume string ignored",
			Command:     test.Command("run", "--rm", "-v", "", testutil.CommonImage),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
	}

	testCase.Run(t)
}

func TestRunVolumeWithDriveRootDestination(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "-d", "--name", data.Identifier(),
			"-v", data.Temp().Dir()+`:C:\.`, testutil.CommonImage)
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeGenericFail,
			Errors:   []error{errors.New("destination path (c:\\\\) cannot be 'c:' or 'c:\\\\'")},
			Output: func(stdout string, t tig.T) {
				psOutput := helpers.Capture("ps", "-a", "--format", "{{.Names}}")
				assert.Assert(t, !strings.Contains(psOutput, data.Identifier()),
					"no container should be created when the volume destination is the drive root")
			},
		}
	}

	testCase.Run(t)
}
