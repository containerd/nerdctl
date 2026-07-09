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
	"testing"

	mobymount "github.com/moby/sys/mount"
	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/mountutil"
	"github.com/containerd/nerdctl/v2/pkg/ociruntimeutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestRunVolume(t *testing.T) {
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
			"-v", fmt.Sprintf("%s:/mnt1", rwDir),
			"-v", fmt.Sprintf("%s:/mnt2:ro", roDir),
			"-v", fmt.Sprintf("%s:/mnt3", rwVolName),
			"-v", fmt.Sprintf("%s:/mnt4:ro", roVolName),
			testutil.AlpineImage,
			"top",
		)

		nerdtest.EnsureContainerStarted(helpers, data.Identifier())

		// Verify rw mounts are writable
		helpers.Ensure("exec", data.Identifier(), "sh", "-exc", "echo -n str1 > /mnt1/file1")
		helpers.Ensure("exec", data.Identifier(), "sh", "-exc", "echo -n str3 > /mnt3/file3")
		// Verify ro mounts are NOT writable
		helpers.Fail("exec", data.Identifier(), "sh", "-exc", "echo -n str2 > /mnt2/file2")
		helpers.Fail("exec", data.Identifier(), "sh", "-exc", "echo -n str4 > /mnt4/file4")

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
					"-v", fmt.Sprintf("%s:/mnt1", data.Labels().Get("rwDir")),
					"-v", fmt.Sprintf("%s:/mnt3", data.Labels().Get("rwVolName")),
					testutil.AlpineImage,
					"cat", "/mnt1/file1", "/mnt3/file3",
				)
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("str1str3")),
		},
		{
			Description: "nested mount ordering",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run",
					"--rm",
					"-v", fmt.Sprintf("%s:/mnt3/mnt1", data.Labels().Get("rwDir")),
					"-v", fmt.Sprintf("%s:/mnt3", data.Labels().Get("rwVolName")),
					testutil.AlpineImage,
					"cat", "/mnt3/mnt1/file1", "/mnt3/file3",
				)
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("str1str3")),
		},
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
		helpers.Anyhow("volume", "rm", "-f", data.Identifier("rw"))
		helpers.Anyhow("volume", "rm", "-f", data.Identifier("ro"))
	}

	testCase.Run(t)
}

func TestRunAnonymousVolume(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.SubTests = []*test.Case{
		{
			Description: "anonymous volume with absolute path",
			Command:     test.Command("run", "--rm", "-v", "/foo", testutil.AlpineImage),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
		{
			Description: "named volume with absolute path",
			Command:     test.Command("run", "--rm", "-v", "TestVolume2:/foo", testutil.AlpineImage),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
		{
			Description: "volume name only",
			Command:     test.Command("run", "--rm", "-v", "TestVolume", testutil.AlpineImage),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
		{
			Description: "destination must be absolute path not named volume",
			Command:     test.Command("run", "--rm", "-v", "TestVolume2:TestVolumes", testutil.AlpineImage),
			Expected:    test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
	}

	testCase.Run(t)
}

func TestRunVolumeRelativePath(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.SubTests = []*test.Case{
		{
			Description: "relative source with absolute destination",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("run", "--rm", "-v", "./foo:/mnt/foo", testutil.AlpineImage)
				cmd.WithCwd(data.Temp().Dir())
				return cmd
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
		{
			Description: "relative source only",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("run", "--rm", "-v", "./foo", testutil.AlpineImage)
				cmd.WithCwd(data.Temp().Dir())
				return cmd
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
		{
			Description: "destination must be absolute not relative",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("run", "--rm", "-v", "./foo:./foo", testutil.AlpineImage)
				cmd.WithCwd(data.Temp().Dir())
				return cmd
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
	}

	testCase.Run(t)
}

func TestRunAnonymousVolumeWithTypeMountFlag(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Command = test.Command("run", "--rm", "--mount", "type=volume,dst=/foo", testutil.AlpineImage,
		"mountpoint", "-q", "/foo")
	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, nil)

	testCase.Run(t)
}

func TestRunAnonymousVolumeWithBuild(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = nerdtest.Build

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		dockerfile := fmt.Sprintf(`FROM %s
VOLUME /foo
        `, testutil.AlpineImage)

		data.Temp().Save(dockerfile, "Dockerfile")
		helpers.Ensure("build", "-t", data.Identifier("img"), data.Temp().Path())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "--rm", "-v", "/foo", testutil.AlpineImage,
			"mountpoint", "-q", "/foo")
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, nil)

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rmi", data.Identifier("img"))
		helpers.Anyhow("builder", "prune", "--all", "--force")
	}

	testCase.Run(t)
}

func TestRunCopyingUpInitialContentsOnVolume(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = nerdtest.Build
	testCase.NoParallel = true

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		dockerfile := fmt.Sprintf(`FROM %s
RUN mkdir -p /mnt && echo hi > /mnt/initial_file
CMD ["cat", "/mnt/initial_file"]
        `, testutil.AlpineImage)

		data.Temp().Save(dockerfile, "Dockerfile")
		imgName := data.Identifier("img")
		helpers.Ensure("build", "-t", imgName, data.Temp().Path())

		volName := data.Identifier("vol")
		helpers.Ensure("volume", "create", volName)

		data.Labels().Set("img", imgName)
		data.Labels().Set("vol", volName)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "without volume flag",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", data.Labels().Get("img"))
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("hi\n")),
		},
		{
			Description: "with anonymous volume",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "-v", "/mnt", "--rm", data.Labels().Get("img"))
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("hi\n")),
		},
		{
			Description: "with named volume",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "-v", data.Labels().Get("vol")+":/mnt", "--rm", data.Labels().Get("img"))
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("hi\n")),
		},
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("volume", "rm", data.Labels().Get("vol"))
		helpers.Anyhow("rmi", data.Labels().Get("img"))
		helpers.Anyhow("builder", "prune", "--all", "--force")
	}

	testCase.Run(t)
}

func TestRunCopyingUpInitialContentsOnDockerfileVolume(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = nerdtest.Build
	testCase.NoParallel = true

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		dockerfile := fmt.Sprintf(`FROM %s
RUN mkdir -p /mnt && echo hi > /mnt/initial_file
VOLUME /mnt
CMD ["cat", "/mnt/initial_file"]
        `, testutil.AlpineImage)

		data.Temp().Save(dockerfile, "Dockerfile")
		imgName := data.Identifier("img")
		helpers.Ensure("build", "-t", imgName, data.Temp().Path())

		volName := data.Identifier("vol")
		helpers.Ensure("volume", "create", volName)

		data.Labels().Set("img", imgName)
		data.Labels().Set("vol", volName)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "anonymous volume from Dockerfile VOLUME",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", data.Labels().Get("img"))
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("hi\n")),
		},
		{
			Description: "anonymous volume with -v flag",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "-v", "/mnt", "--rm", data.Labels().Get("img"))
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("hi\n")),
		},
		{
			Description: "named volume copies initial contents",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "-v", data.Labels().Get("vol")+":/mnt", "--rm", data.Labels().Get("img"))
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("hi\n")),
		},
		{
			Description: "bind mount does not copy initial contents",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "-v", fmt.Sprintf("%s:/mnt", data.Temp().Dir("bindmnt")), "--rm", data.Labels().Get("img"))
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("volume", "rm", data.Labels().Get("vol"))
		helpers.Anyhow("rmi", data.Labels().Get("img"))
		helpers.Anyhow("builder", "prune", "--all", "--force")
	}

	testCase.Run(t)
}

func TestRunCopyingUpInitialContentsOnVolumeShouldRetainSymlink(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = nerdtest.Build
	testCase.NoParallel = true

	const expected = "../../../../../../../../../../../../../../../../../../etc/passwd\n"

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		dockerfile := fmt.Sprintf(`FROM %s
RUN ln -s ../../../../../../../../../../../../../../../../../../etc/passwd /mnt/passwd
VOLUME /mnt
CMD ["readlink", "/mnt/passwd"]
        `, testutil.AlpineImage)

		data.Temp().Save(dockerfile, "Dockerfile")
		imgName := data.Identifier("img")
		helpers.Ensure("build", "-t", imgName, data.Temp().Path())

		data.Labels().Set("img", imgName)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "without explicit volume flag",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", data.Labels().Get("img"))
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals(expected)),
		},
		{
			Description: "with anonymous volume flag",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "-v", "/mnt", "--rm", data.Labels().Get("img"))
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals(expected)),
		},
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rmi", data.Labels().Get("img"))
		helpers.Anyhow("builder", "prune", "--all", "--force")
	}

	testCase.Run(t)
}

func TestRunCopyingUpInitialContentsShouldNotResetTheCopiedContents(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = nerdtest.Build

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		dockerfile := fmt.Sprintf(`FROM %s
RUN echo -n "rev0" > /mnt/file
`, testutil.AlpineImage)

		data.Temp().Save(dockerfile, "Dockerfile")
		helpers.Ensure("build", "-t", data.Identifier("img"), data.Temp().Path())

		helpers.Ensure("volume", "create", data.Identifier("vol"))

		// First run: verify initial content is copied, then modify it
		helpers.Ensure("run", "-d", "--name", data.Identifier(),
			"-v", data.Identifier("vol")+":/mnt",
			data.Identifier("img"), "sleep", nerdtest.Infinity)
		nerdtest.EnsureContainerStarted(helpers, data.Identifier())

		rev0 := helpers.Capture("exec", data.Identifier(), "cat", "/mnt/file")
		assert.Equal(helpers.T(), rev0, "rev0")

		helpers.Ensure("exec", data.Identifier(), "sh", "-euc", `echo -n "rev1" >/mnt/file`)
		helpers.Ensure("rm", "-f", data.Identifier())

		// Second run: volume content should be "rev1", not reset to "rev0"
		helpers.Ensure("run", "-d", "--name", data.Identifier(),
			"-v", data.Identifier("vol")+":/mnt",
			data.Identifier("img"), "sleep", nerdtest.Infinity)
		nerdtest.EnsureContainerStarted(helpers, data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("exec", data.Identifier(), "cat", "/mnt/file")
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("rev1"))

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
		helpers.Anyhow("volume", "rm", data.Identifier("vol"))
		helpers.Anyhow("rmi", data.Identifier("img"))
		helpers.Anyhow("builder", "prune", "--all", "--force")
	}

	testCase.Run(t)
}

func expectMountOptions(allow, deny []string) test.Comparator {
	return func(stdout string, t tig.T) {
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		assert.Assert(t, len(lines) == 1, "expected 1 line, got %d: %q", len(lines), stdout)
		for _, s := range allow {
			assert.Assert(t, strings.Contains(stdout, s), "expected stdout to contain %q, got %q", s, stdout)
		}
		for _, s := range deny {
			assert.Assert(t, !strings.Contains(stdout, s), "expected stdout not to contain %q, got %q", s, stdout)
		}
	}
}

func TestRunTmpfs(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.SubTests = []*test.Case{
		{
			Description: "tmpfs default options",
			Command:     test.Command("run", "--rm", "--tmpfs", "/tmp", testutil.AlpineImage, "grep", "/tmp", "/proc/mounts"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expectMountOptions([]string{"rw", "nosuid", "nodev", "noexec"}, nil),
				}
			},
		},
		{
			Description: "tmpfs with size and exec",
			Command:     test.Command("run", "--rm", "--tmpfs", "/tmp:size=64m,exec", testutil.AlpineImage, "grep", "/tmp", "/proc/mounts"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expectMountOptions([]string{"rw", "nosuid", "nodev", "size=65536k"}, []string{"noexec"}),
				}
			},
		},
		{
			// https://github.com/containerd/nerdctl/issues/594
			Description: "tmpfs on /dev/shm with rw exec and size",
			Command:     test.Command("run", "--rm", "--tmpfs", "/dev/shm:rw,exec,size=1g", testutil.AlpineImage, "grep", "/dev/shm", "/proc/mounts"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expectMountOptions([]string{"rw", "nosuid", "nodev", "size=1048576k"}, []string{"noexec"}),
				}
			},
		},
	}

	testCase.Run(t)
}

func TestRunBindMountTmpfs(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.SubTests = []*test.Case{
		{
			Description: "mount type tmpfs default",
			Command:     test.Command("run", "--rm", "--mount", "type=tmpfs,target=/tmp", testutil.AlpineImage, "grep", "/tmp", "/proc/mounts"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expectMountOptions([]string{"rw", "nosuid", "nodev", "noexec"}, nil),
				}
			},
		},
		{
			Description: "mount type tmpfs with size",
			Command:     test.Command("run", "--rm", "--mount", "type=tmpfs,target=/tmp,tmpfs-size=64m", testutil.AlpineImage, "grep", "/tmp", "/proc/mounts"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expectMountOptions([]string{"rw", "nosuid", "nodev", "size=65536k"}, nil),
				}
			},
		},
	}

	testCase.Run(t)
}

func mountExistsWithOpt(mountPoint, mountOpt string) test.Comparator {
	return func(stdout string, t tig.T) {
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		mountOutput := []string{}
		for _, line := range lines {
			if strings.Contains(line, mountPoint) {
				mountOutput = strings.Split(line, " ")
				break
			}
		}

		assert.Assert(t, len(mountOutput) > 0, "we should have found the mount point in /proc/mounts")
		assert.Assert(t, len(mountOutput) >= 4, "invalid format for mount line")

		options := strings.Split(mountOutput[3], ",")

		found := false
		for _, opt := range options {
			if mountOpt == opt {
				found = true
				break
			}
		}

		assert.Assert(t, found, "mount option %s not found", mountOpt)
	}
}

func TestRunBindMountBind(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		// Run a container with bind mount directories, one rw, the other ro
		rwDir := data.Temp().Dir("rw")
		roDir := data.Temp().Dir("ro")

		helpers.Ensure(
			"run",
			"-d",
			"--name", data.Identifier("container"),
			"--mount", fmt.Sprintf("type=bind,src=%s,target=/mntrw", rwDir),
			"--mount", fmt.Sprintf("type=bind,src=%s,target=/mntro,ro", roDir),
			testutil.AlpineImage,
			"top",
		)

		nerdtest.EnsureContainerStarted(helpers, data.Identifier("container"))

		// Save host rwDir location and container id for subtests
		data.Labels().Set("container", data.Identifier("container"))
		data.Labels().Set("rwDir", rwDir)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "ensure we cannot write to ro mount",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("exec", data.Labels().Get("container"), "sh", "-exc", "echo -n failure > /mntro/file")
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			Description: "ensure we can write to rw, and read it back from another container mounting the same target",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("exec", data.Labels().Get("container"), "sh", "-exc", "echo -n success > /mntrw/file")
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command(
					"run",
					"--rm",
					"--mount", fmt.Sprintf("type=bind,src=%s,target=/mntrw", data.Labels().Get("rwDir")),
					testutil.AlpineImage,
					"cat", "/mntrw/file",
				)
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("success")),
		},
		{
			Description: "Check that mntrw is seen in /proc/mounts",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("exec", data.Labels().Get("container"), "cat", "/proc/mounts")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.All(
						// Ensure we have mntrw in the mount list
						mountExistsWithOpt("/mntrw", "rw"),
						mountExistsWithOpt("/mntro", "ro"),
					),
				}
			},
		},
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier("container"))
	}

	testCase.Run(t)
}

func expectFindmntLines(expectedLines int, expectedPrefix string) test.Comparator {
	return func(stdout string, t tig.T) {
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		assert.Assert(t, len(lines) == expectedLines, "expected %d line(s), got %d: %q", expectedLines, len(lines), stdout)
		assert.Assert(t, strings.HasPrefix(lines[0], expectedPrefix), "expected mount %s, got %q", expectedPrefix, lines[0])
	}
}

func TestRunMountBindMode(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = nerdtest.Rootful

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		tmpDir1 := data.Temp().Dir("rw")
		tmpDir1Mnt := data.Temp().Dir("rw", "mnt")
		tmpDir2 := data.Temp().Dir("ro")

		err := mobymount.Mount(tmpDir2, tmpDir1Mnt, "none", "bind,ro")
		assert.NilError(helpers.T(), err, "failed to mount")

		data.Labels().Set("tmpDir1", tmpDir1)
		data.Labels().Set("tmpDir1Mnt", tmpDir1Mnt)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "bind-recursive disabled hides submounts",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run",
					"--rm",
					"--mount", fmt.Sprintf("type=bind,bind-recursive=disabled,src=%s,target=/mnt1", data.Labels().Get("tmpDir1")),
					testutil.AlpineImage,
					"sh", "-euxc", "apk add findmnt -q && findmnt -nR /mnt1",
				)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expectFindmntLines(1, "/mnt1"),
				}
			},
		},
		{
			Description: "bind-recursive enabled shows submounts",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run",
					"--rm",
					"--mount", fmt.Sprintf("type=bind,bind-recursive=enabled,src=%s,target=/mnt1", data.Labels().Get("tmpDir1")),
					testutil.AlpineImage,
					"sh", "-euxc", "apk add findmnt -q && findmnt -nR /mnt1",
				)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expectFindmntLines(2, "/mnt1"),
				}
			},
		},
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		mntPath := data.Labels().Get("tmpDir1Mnt")
		if mntPath != "" {
			_ = mobymount.Unmount(mntPath)
		}
	}

	testCase.Run(t)
}

func TestRunVolumeBindMode(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.All(nerdtest.Rootful, require.Not(nerdtest.Docker))

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		tmpDir1 := data.Temp().Dir("rw")
		tmpDir1Mnt := data.Temp().Dir("rw", "mnt")
		tmpDir2 := data.Temp().Dir("ro")

		err := mobymount.Mount(tmpDir2, tmpDir1Mnt, "none", "bind,ro")
		assert.NilError(helpers.T(), err, "failed to mount")

		data.Labels().Set("tmpDir1", tmpDir1)
		data.Labels().Set("tmpDir1Mnt", tmpDir1Mnt)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "bind mode hides submounts",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run",
					"--rm",
					"-v", fmt.Sprintf("%s:/mnt1:bind", data.Labels().Get("tmpDir1")),
					testutil.AlpineImage,
					"sh", "-euxc", "apk add findmnt -q && findmnt -nR /mnt1",
				)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expectFindmntLines(1, "/mnt1"),
				}
			},
		},
		{
			Description: "rbind mode shows submounts",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run",
					"--rm",
					"-v", fmt.Sprintf("%s:/mnt1:rbind", data.Labels().Get("tmpDir1")),
					testutil.AlpineImage,
					"sh", "-euxc", "apk add findmnt -q && findmnt -nR /mnt1",
				)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expectFindmntLines(2, "/mnt1"),
				}
			},
		},
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		mntPath := data.Labels().Get("tmpDir1Mnt")
		if mntPath != "" {
			_ = mobymount.Unmount(mntPath)
		}
	}

	testCase.Run(t)
}

// requiresRRO requires that the kernel and the default OCI runtime support
// recursive read-only (RRO) mounts.
var requiresRRO = &test.Requirement{
	Check: func(data test.Data, helpers test.Helpers) (bool, string) {
		if err := ociruntimeutil.SupportsRecursivelyReadOnly(""); err != nil {
			return false, fmt.Sprintf("recursive read-only mounts are not supported: %v", err)
		}
		return true, "recursive read-only mounts are supported"
	},
}

// setupBindMountWithSubmount creates a temp directory ("top") containing a
// writable submount ("top/mnt"), for testing recursive read-only (RRO) mounts,
// and stores the path of the top directory in the "top" label.
func setupBindMountWithSubmount(data test.Data, helpers test.Helpers) {
	top := data.Temp().Dir("top")
	topMnt := data.Temp().Dir("top", "mnt")
	sub := data.Temp().Dir("sub")
	assert.NilError(helpers.T(), mobymount.Mount(sub, topMnt, "none", "bind"))
	data.Labels().Set("top", top)
}

func cleanupBindMountWithSubmount(data test.Data, helpers test.Helpers) {
	if top := data.Labels().Get("top"); top != "" {
		topMnt := filepath.Join(top, "mnt")
		if err := mobymount.Unmount(topMnt); err != nil {
			helpers.T().Log(fmt.Sprintf("failed to unmount %q: %v", topMnt, err))
		}
	}
}

// TestRunBindMountRecursiveReadOnly tests that read-only bind mounts are
// recursively read-only when the kernel and the OCI runtime support it
// (Docker v25 behavior), and that the mode is customizable with the
// `bind-recursive` option of `--mount`.
func TestRunBindMountRecursiveReadOnly(t *testing.T) {
	testCase := nerdtest.Setup()

	// The test creates a bind mount on the host, in a mount namespace shared
	// with the daemon. With the rootless harness, the test process runs on the
	// host, while the daemon runs inside the mount namespace of RootlessKit,
	// so the mount would not be visible to the daemon.
	testCase.Require = require.All(
		require.Not(nerdtest.Rootless),
		requiresRRO,
	)

	testCase.Setup = setupBindMountWithSubmount

	testCase.SubTests = []*test.Case{
		{
			Description: "-v :ro is recursively read-only by default",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm",
					"-v", data.Labels().Get("top")+":/mnt1:ro",
					testutil.AlpineImage,
					"sh", "-euxc", "! touch /mnt1/mnt/file")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
		{
			Description: "--mount readonly is recursively read-only by default",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm",
					"--mount", fmt.Sprintf("type=bind,src=%s,target=/mnt1,readonly", data.Labels().Get("top")),
					testutil.AlpineImage,
					"sh", "-euxc", "! touch /mnt1/mnt/file")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
		{
			Description: "bind-recursive=writable keeps the submounts writable (Docker v24 behavior)",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm",
					"--mount", fmt.Sprintf("type=bind,src=%s,target=/mnt1,readonly,bind-recursive=writable", data.Labels().Get("top")),
					testutil.AlpineImage,
					"sh", "-euxc", "! touch /mnt1/file && touch /mnt1/mnt/file")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
		{
			Description: "bind-recursive=readonly forces the recursive read-only mount",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm",
					"--mount", fmt.Sprintf("type=bind,src=%s,target=/mnt1,readonly,bind-propagation=rprivate,bind-recursive=readonly", data.Labels().Get("top")),
					testutil.AlpineImage,
					"sh", "-euxc", "! touch /mnt1/mnt/file")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
	}

	testCase.Cleanup = cleanupBindMountWithSubmount

	testCase.Run(t)
}

// TestRunBindMountDeprecatedRRO tests the deprecated `rro` option of `-v` and
// `--mount`, which predates the `bind-recursive=readonly` option of Docker v25.
func TestRunBindMountDeprecatedRRO(t *testing.T) {
	testCase := nerdtest.Setup()

	// See TestRunBindMountRecursiveReadOnly for the rootless restriction.
	testCase.Require = require.All(
		require.Not(nerdtest.Docker),
		require.Not(nerdtest.Rootless),
		requiresRRO,
	)

	testCase.Setup = setupBindMountWithSubmount

	testCase.SubTests = []*test.Case{
		{
			Description: "-v :rro",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm",
					"-v", data.Labels().Get("top")+":/mnt1:rro,rprivate",
					testutil.AlpineImage,
					"sh", "-euxc", "! touch /mnt1/mnt/file")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
		{
			Description: "--mount rro",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm",
					"--mount", fmt.Sprintf("type=bind,src=%s,target=/mnt1,rro,bind-propagation=rprivate", data.Labels().Get("top")),
					testutil.AlpineImage,
					"sh", "-euxc", "! touch /mnt1/mnt/file")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
	}

	testCase.Cleanup = cleanupBindMountWithSubmount

	testCase.Run(t)
}

func TestRunBindMountPropagation(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.T().Skip("This test is currently broken. See https://github.com/containerd/nerdctl/issues/3404")
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "rshared propagation",
			Setup: func(data test.Data, helpers test.Helpers) {
				rwDir := data.Temp().Dir("rshared")
				data.Labels().Set("rshared-rwDir", rwDir)

				helpers.Ensure("run", "-d", "--privileged",
					"--name", data.Identifier("rshared"),
					"--mount", fmt.Sprintf("type=bind,src=%s,target=/mnt1,bind-propagation=rshared", rwDir),
					testutil.AlpineImage, "top")

				helpers.Ensure("run", "-d", "--privileged",
					"--name", data.Identifier("rshared-replica"),
					"--mount", fmt.Sprintf("type=bind,src=%s,target=/mnt1,bind-propagation=rshared", rwDir),
					testutil.AlpineImage, "top")

				// mount in the first container
				helpers.Ensure("exec", data.Identifier("rshared"), "sh", "-exc",
					"mkdir /app && mkdir /mnt1/replica && mount --bind /app /mnt1/replica && echo -n toreplica > /app/foo.txt")

				// mount in the second container
				helpers.Ensure("exec", data.Identifier("rshared-replica"), "sh", "-exc", "mkdir /bar && mkdir /mnt1/bar")
				helpers.Ensure("exec", data.Identifier("rshared-replica"), "sh", "-exc", "mount --bind /bar /mnt1/bar")
				helpers.Ensure("exec", data.Identifier("rshared-replica"), "sh", "-exc", "echo -n fromreplica > /bar/bar.txt")
			},
			SubTests: []*test.Case{
				{
					Description: "replica can get sub-mounts from original",
					Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
						return helpers.Command("exec", data.Identifier("rshared-replica"), "cat", "/mnt1/replica/foo.txt")
					},
					Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("toreplica")),
				},
				{
					Description: "sub-mounts from replica propagated to original",
					Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
						return helpers.Command("exec", data.Identifier("rshared"), "cat", "/mnt1/bar/bar.txt")
					},
					Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("fromreplica")),
				},
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("exec", data.Identifier("rshared-replica"), "sh", "-exc", "umount /mnt1/bar")
				helpers.Anyhow("exec", data.Identifier("rshared"), "sh", "-exc", "umount /mnt1/replica")
				helpers.Anyhow("rm", "-f", data.Identifier("rshared"))
				helpers.Anyhow("rm", "-f", data.Identifier("rshared-replica"))
			},
		},
		{
			Description: "rslave propagation",
			Setup: func(data test.Data, helpers test.Helpers) {
				rwDir := data.Temp().Dir("rslave")

				helpers.Ensure("run", "-d", "--privileged",
					"--name", data.Identifier("rslave"),
					"--mount", fmt.Sprintf("type=bind,src=%s,target=/mnt1,bind-propagation=rshared", rwDir),
					testutil.AlpineImage, "top")

				helpers.Ensure("run", "-d", "--privileged",
					"--name", data.Identifier("rslave-replica"),
					"--mount", fmt.Sprintf("type=bind,src=%s,target=/mnt1,bind-propagation=rslave", rwDir),
					testutil.AlpineImage, "top")

				helpers.Ensure("exec", data.Identifier("rslave"), "sh", "-exc",
					"mkdir /app && mkdir /mnt1/replica && mount --bind /app /mnt1/replica && echo -n toreplica > /app/foo.txt")

				helpers.Ensure("exec", data.Identifier("rslave-replica"), "sh", "-exc", "mkdir /bar && mkdir /mnt1/bar")
				helpers.Ensure("exec", data.Identifier("rslave-replica"), "sh", "-exc", "mount --bind /bar /mnt1/bar")
				helpers.Ensure("exec", data.Identifier("rslave-replica"), "sh", "-exc", "echo -n fromreplica > /bar/bar.txt")
			},
			SubTests: []*test.Case{
				{
					Description: "replica can get sub-mounts from original",
					Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
						return helpers.Command("exec", data.Identifier("rslave-replica"), "cat", "/mnt1/replica/foo.txt")
					},
					Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("toreplica")),
				},
				{
					Description: "sub-mounts from replica not propagated to original",
					Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
						return helpers.Command("exec", data.Identifier("rslave"), "cat", "/mnt1/bar/bar.txt")
					},
					Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
				},
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("exec", data.Identifier("rslave-replica"), "sh", "-exc", "umount /mnt1/bar")
				helpers.Anyhow("exec", data.Identifier("rslave"), "sh", "-exc", "umount /mnt1/replica")
				helpers.Anyhow("rm", "-f", data.Identifier("rslave"))
				helpers.Anyhow("rm", "-f", data.Identifier("rslave-replica"))
			},
		},
		{
			Description: "rprivate propagation",
			Setup: func(data test.Data, helpers test.Helpers) {
				rwDir := data.Temp().Dir("rprivate")

				helpers.Ensure("run", "-d", "--privileged",
					"--name", data.Identifier("rprivate"),
					"--mount", fmt.Sprintf("type=bind,src=%s,target=/mnt1,bind-propagation=rshared", rwDir),
					testutil.AlpineImage, "top")

				helpers.Ensure("run", "-d", "--privileged",
					"--name", data.Identifier("rprivate-replica"),
					"--mount", fmt.Sprintf("type=bind,src=%s,target=/mnt1,bind-propagation=rprivate", rwDir),
					testutil.AlpineImage, "top")

				helpers.Ensure("exec", data.Identifier("rprivate"), "sh", "-exc",
					"mkdir /app && mkdir /mnt1/replica && mount --bind /app /mnt1/replica && echo -n toreplica > /app/foo.txt")

				helpers.Ensure("exec", data.Identifier("rprivate-replica"), "sh", "-exc", "mkdir /bar && mkdir /mnt1/bar")
				helpers.Ensure("exec", data.Identifier("rprivate-replica"), "sh", "-exc", "mount --bind /bar /mnt1/bar")
				helpers.Ensure("exec", data.Identifier("rprivate-replica"), "sh", "-exc", "echo -n fromreplica > /bar/bar.txt")
			},
			SubTests: []*test.Case{
				{
					Description: "replica cannot get sub-mounts from original",
					Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
						return helpers.Command("exec", data.Identifier("rprivate-replica"), "cat", "/mnt1/replica/foo.txt")
					},
					Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
				},
				{
					Description: "sub-mounts from replica not propagated to original",
					Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
						return helpers.Command("exec", data.Identifier("rprivate"), "cat", "/mnt1/bar/bar.txt")
					},
					Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
				},
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("exec", data.Identifier("rprivate-replica"), "sh", "-exc", "umount /mnt1/bar")
				helpers.Anyhow("exec", data.Identifier("rprivate"), "sh", "-exc", "umount /mnt1/replica")
				helpers.Anyhow("rm", "-f", data.Identifier("rprivate"))
				helpers.Anyhow("rm", "-f", data.Identifier("rprivate-replica"))
			},
		},
		{
			Description: "default propagation",
			Setup: func(data test.Data, helpers test.Helpers) {
				rwDir := data.Temp().Dir("default")

				helpers.Ensure("run", "-d", "--privileged",
					"--name", data.Identifier("default"),
					"--mount", fmt.Sprintf("type=bind,src=%s,target=/mnt1,bind-propagation=rshared", rwDir),
					testutil.AlpineImage, "top")

				helpers.Ensure("run", "-d", "--privileged",
					"--name", data.Identifier("default-replica"),
					"--mount", fmt.Sprintf("type=bind,src=%s,target=/mnt1", rwDir),
					testutil.AlpineImage, "top")

				helpers.Ensure("exec", data.Identifier("default"), "sh", "-exc",
					"mkdir /app && mkdir /mnt1/replica && mount --bind /app /mnt1/replica && echo -n toreplica > /app/foo.txt")

				helpers.Ensure("exec", data.Identifier("default-replica"), "sh", "-exc", "mkdir /bar && mkdir /mnt1/bar")
				helpers.Ensure("exec", data.Identifier("default-replica"), "sh", "-exc", "mount --bind /bar /mnt1/bar")
				helpers.Ensure("exec", data.Identifier("default-replica"), "sh", "-exc", "echo -n fromreplica > /bar/bar.txt")
			},
			SubTests: []*test.Case{
				{
					Description: "replica cannot get sub-mounts from original",
					Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
						return helpers.Command("exec", data.Identifier("default-replica"), "cat", "/mnt1/replica/foo.txt")
					},
					Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
				},
				{
					Description: "sub-mounts from replica not propagated to original",
					Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
						return helpers.Command("exec", data.Identifier("default"), "cat", "/mnt1/bar/bar.txt")
					},
					Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
				},
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("exec", data.Identifier("default-replica"), "sh", "-exc", "umount /mnt1/bar")
				helpers.Anyhow("exec", data.Identifier("default"), "sh", "-exc", "umount /mnt1/replica")
				helpers.Anyhow("rm", "-f", data.Identifier("default"))
				helpers.Anyhow("rm", "-f", data.Identifier("default-replica"))
			},
		},
	}

	testCase.Run(t)
}

func TestRunVolumesFrom(t *testing.T) {
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
			"--name", data.Identifier("from"),
			"-v", fmt.Sprintf("%s:/mnt1", rwDir),
			"-v", fmt.Sprintf("%s:/mnt2:ro", roDir),
			"-v", fmt.Sprintf("%s:/mnt3", rwVolName),
			"-v", fmt.Sprintf("%s:/mnt4:ro", roVolName),
			testutil.AlpineImage,
			"top",
		)

		nerdtest.EnsureContainerStarted(helpers, data.Identifier("from"))

		helpers.Ensure("run",
			"-d",
			"--name", data.Identifier("to"),
			"--volumes-from", data.Identifier("from"),
			testutil.AlpineImage,
			"top",
		)

		nerdtest.EnsureContainerStarted(helpers, data.Identifier("to"))

		// Verify rw mounts are writable via volumes-from container
		helpers.Ensure("exec", data.Identifier("to"), "sh", "-exc", "echo -n str1 > /mnt1/file1")
		helpers.Ensure("exec", data.Identifier("to"), "sh", "-exc", "echo -n str3 > /mnt3/file3")
		// Verify ro mounts are NOT writable
		helpers.Fail("exec", data.Identifier("to"), "sh", "-exc", "echo -n str2 > /mnt2/file2")
		helpers.Fail("exec", data.Identifier("to"), "sh", "-exc", "echo -n str4 > /mnt4/file4")

		helpers.Ensure("rm", "-f", data.Identifier("to"))
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run",
			"--rm",
			"--volumes-from", data.Identifier("from"),
			testutil.AlpineImage,
			"cat", "/mnt1/file1", "/mnt3/file3",
		)
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("str1str3"))

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier("to"))
		helpers.Anyhow("rm", "-f", data.Identifier("from"))
		helpers.Anyhow("volume", "rm", "-f", data.Identifier("rw"))
		helpers.Anyhow("volume", "rm", "-f", data.Identifier("ro"))
	}

	testCase.Run(t)
}

func TestBindMountWhenHostFolderDoesNotExist(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.SubTests = []*test.Case{
		{
			Description: "bind mount with -v auto-creates host directory",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				hp := data.Temp().Path("v-does-not-exist")
				data.Labels().Set("hostPath", hp)
				return helpers.Command("run", "--name", data.Identifier("v"), "-d",
					"-v", fmt.Sprintf("%s:/tmp", hp),
					testutil.AlpineImage)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, t tig.T) {
						_, err := os.Stat(data.Labels().Get("hostPath"))
						assert.NilError(t, err, "host directory should exist after -v mount")
					},
				}
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier("v"))
			},
		},
		{
			Description: "bind mount with --mount does not auto-create host directory",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				hp := data.Temp().Path("mount-does-not-exist")
				data.Labels().Set("hostPath", hp)
				return helpers.Command("run", "--name", data.Identifier("mount"), "-d",
					"--mount", fmt.Sprintf("type=bind, source=%s, target=/tmp", hp),
					testutil.AlpineImage)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeGenericFail,
					Output: func(stdout string, t tig.T) {
						_, err := os.Stat(data.Labels().Get("hostPath"))
						assert.ErrorIs(t, err, os.ErrNotExist, "host directory should NOT exist after --mount failure")
					},
				}
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier("mount"))
			},
		},
	}

	testCase.Run(t)
}

func TestRunVolumeWithRootDestination(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "-d", "--name", data.Identifier(),
			"-v", data.Temp().Dir()+":/", testutil.AlpineImage)
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeGenericFail,
			Errors:   []error{mountutil.ErrVolumeTargetIsRoot},
			Output: func(stdout string, t tig.T) {
				psOutput := helpers.Capture("ps", "-a", "--format", "{{.Names}}")
				assert.Assert(t, !strings.Contains(psOutput, data.Identifier()),
					"no container should be created when the volume destination is '/'")
			},
		}
	}

	testCase.Run(t)
}
