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
	"testing"

	"github.com/containerd/containerd/mount"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/containerd/nerdctl/pkg/testutil"
	mobymount "github.com/moby/sys/mount"
	"gotest.tools/v3/assert"
)

func TestRunVolume(t *testing.T) {
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
		"-v", fmt.Sprintf("%s:/mnt1", rwDir),
		"-v", fmt.Sprintf("%s:/mnt2:ro", roDir),
		"-v", fmt.Sprintf("%s:/mnt3", rwVolName),
		"-v", fmt.Sprintf("%s:/mnt4:ro", roVolName),
		testutil.AlpineImage,
		"top",
	).AssertOK()
	base.Cmd("exec", containerName, "sh", "-exc", "echo -n str1 > /mnt1/file1").AssertOK()
	base.Cmd("exec", containerName, "sh", "-exc", "echo -n str2 > /mnt2/file2").AssertFail()
	base.Cmd("exec", containerName, "sh", "-exc", "echo -n str3 > /mnt3/file3").AssertOK()
	base.Cmd("exec", containerName, "sh", "-exc", "echo -n str4 > /mnt4/file4").AssertFail()
	base.Cmd("rm", "-f", containerName).AssertOK()
	base.Cmd("run",
		"--rm",
		"-v", fmt.Sprintf("%s:/mnt1", rwDir),
		"-v", fmt.Sprintf("%s:/mnt3", rwVolName),
		testutil.AlpineImage,
		"cat", "/mnt1/file1", "/mnt3/file3",
	).AssertOutExactly("str1str3")
}

func TestRunAnonymousVolume(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	base.Cmd("run", "--rm", "-v", "/foo", testutil.AlpineImage,
		"mountpoint", "-q", "/foo").AssertOK()
}

func TestRunAnonymousVolumeWithTypeMountFlag(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	base.Cmd("run", "--rm", "--mount", "type=volume,dst=/foo", testutil.AlpineImage,
		"mountpoint", "-q", "/foo").AssertOK()
}

func TestRunAnonymousVolumeWithBuild(t *testing.T) {
	t.Parallel()
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
VOLUME /foo
        `, testutil.AlpineImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", imageName, buildCtx).AssertOK()
	base.Cmd("run", "--rm", "-v", "/foo", testutil.AlpineImage,
		"mountpoint", "-q", "/foo").AssertOK()
}

func TestRunCopyingUpInitialContentsOnVolume(t *testing.T) {
	t.Parallel()
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()
	volName := testutil.Identifier(t) + "-vol"
	defer base.Cmd("volume", "rm", volName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
RUN mkdir -p /mnt && echo hi > /mnt/initial_file
CMD ["cat", "/mnt/initial_file"]
        `, testutil.AlpineImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", imageName, buildCtx).AssertOK()

	//AnonymousVolume
	base.Cmd("run", "--rm", imageName).AssertOutExactly("hi\n")
	base.Cmd("run", "-v", "/mnt", "--rm", imageName).AssertOutExactly("hi\n")

	//NamedVolume should be automatically created
	base.Cmd("run", "-v", volName+":/mnt", "--rm", imageName).AssertOutExactly("hi\n")
}

func TestRunCopyingUpInitialContentsOnDockerfileVolume(t *testing.T) {
	t.Parallel()
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()
	volName := testutil.Identifier(t) + "-vol"
	defer base.Cmd("volume", "rm", volName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
RUN mkdir -p /mnt && echo hi > /mnt/initial_file
VOLUME /mnt
CMD ["cat", "/mnt/initial_file"]
        `, testutil.AlpineImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", imageName, buildCtx).AssertOK()
	//AnonymousVolume
	base.Cmd("run", "--rm", imageName).AssertOutExactly("hi\n")
	base.Cmd("run", "-v", "/mnt", "--rm", imageName).AssertOutExactly("hi\n")

	//NamedVolume
	base.Cmd("volume", "create", volName).AssertOK()
	base.Cmd("run", "-v", volName+":/mnt", "--rm", imageName).AssertOutExactly("hi\n")

	//mount bind
	tmpDir, err := os.MkdirTemp(t.TempDir(), "hostDir")
	assert.NilError(t, err)

	base.Cmd("run", "-v", fmt.Sprintf("%s:/mnt", tmpDir), "--rm", imageName).AssertFail()
}

func TestRunCopyingUpInitialContentsOnVolumeShouldRetainSymlink(t *testing.T) {
	t.Parallel()
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
RUN ln -s ../../../../../../../../../../../../../../../../../../etc/passwd /mnt/passwd
VOLUME /mnt
CMD ["readlink", "/mnt/passwd"]
        `, testutil.AlpineImage)
	const expected = "../../../../../../../../../../../../../../../../../../etc/passwd\n"

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", imageName, buildCtx).AssertOK()

	base.Cmd("run", "--rm", imageName).AssertOutExactly(expected)
	base.Cmd("run", "-v", "/mnt", "--rm", imageName).AssertOutExactly(expected)
}

func TestRunCopyingUpInitialContentsShouldNotResetTheCopiedContents(t *testing.T) {
	t.Parallel()
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	tID := testutil.Identifier(t)
	imageName := tID + "-img"
	volumeName := tID + "-vol"
	containerName := tID
	defer func() {
		base.Cmd("rm", "-f", containerName).Run()
		base.Cmd("volume", "rm", volumeName).Run()
		base.Cmd("rmi", imageName).Run()
	}()

	dockerfile := fmt.Sprintf(`FROM %s
RUN echo -n "rev0" > /mnt/file
`, testutil.AlpineImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", imageName, buildCtx).AssertOK()

	base.Cmd("volume", "create", volumeName)
	runContainer := func() {
		base.Cmd("run", "-d", "--name", containerName, "-v", volumeName+":/mnt", imageName, "sleep", "infinity").AssertOK()
	}
	runContainer()
	base.EnsureContainerStarted(containerName)
	base.Cmd("exec", containerName, "cat", "/mnt/file").AssertOutExactly("rev0")
	base.Cmd("exec", containerName, "sh", "-euc", "echo -n \"rev1\" >/mnt/file").AssertOK()
	base.Cmd("rm", "-f", containerName).AssertOK()
	runContainer()
	base.EnsureContainerStarted(containerName)
	base.Cmd("exec", containerName, "cat", "/mnt/file").AssertOutExactly("rev1")
}

func TestRunTmpfs(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	f := func(allow, deny []string) func(stdout string) error {
		return func(stdout string) error {
			lines := strings.Split(strings.TrimSpace(stdout), "\n")
			if len(lines) != 1 {
				return fmt.Errorf("expected 1 lines, got %q", stdout)
			}
			for _, s := range allow {
				if !strings.Contains(stdout, s) {
					return fmt.Errorf("expected stdout to contain %q, got %q", s, stdout)
				}
			}
			for _, s := range deny {
				if strings.Contains(stdout, s) {
					return fmt.Errorf("expected stdout not to contain %q, got %q", s, stdout)
				}
			}
			return nil
		}
	}
	base.Cmd("run", "--rm", "--tmpfs", "/tmp", testutil.AlpineImage, "grep", "/tmp", "/proc/mounts").AssertOutWithFunc(f([]string{"rw", "nosuid", "nodev", "noexec"}, nil))
	base.Cmd("run", "--rm", "--tmpfs", "/tmp:size=64m,exec", testutil.AlpineImage, "grep", "/tmp", "/proc/mounts").AssertOutWithFunc(f([]string{"rw", "nosuid", "nodev", "size=65536k"}, []string{"noexec"}))
	// for https://github.com/containerd/nerdctl/issues/594
	base.Cmd("run", "--rm", "--tmpfs", "/dev/shm:rw,exec,size=1g", testutil.AlpineImage, "grep", "/dev/shm", "/proc/mounts").AssertOutWithFunc(f([]string{"rw", "nosuid", "nodev", "size=1048576k"}, []string{"noexec"}))
}

func TestRunBindMountTmpfs(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	f := func(allow []string) func(stdout string) error {
		return func(stdout string) error {
			lines := strings.Split(strings.TrimSpace(stdout), "\n")
			if len(lines) != 1 {
				return fmt.Errorf("expected 1 lines, got %q", stdout)
			}
			for _, s := range allow {
				if !strings.Contains(stdout, s) {
					return fmt.Errorf("expected stdout to contain %q, got %q", s, stdout)
				}
			}
			return nil
		}
	}
	base.Cmd("run", "--rm", "--mount", "type=tmpfs,target=/tmp", testutil.AlpineImage, "grep", "/tmp", "/proc/mounts").AssertOutWithFunc(f([]string{"rw", "nosuid", "nodev", "noexec"}))
	base.Cmd("run", "--rm", "--mount", "type=tmpfs,target=/tmp,tmpfs-size=64m", testutil.AlpineImage, "grep", "/tmp", "/proc/mounts").AssertOutWithFunc(f([]string{"rw", "nosuid", "nodev", "size=65536k"}))
}

func TestRunBindMountBind(t *testing.T) {
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

	containerName := tID
	defer base.Cmd("rm", "-f", containerName).AssertOK()
	base.Cmd("run",
		"-d",
		"--name", containerName,
		"--mount", fmt.Sprintf("type=bind,src=%s,target=/mnt1", rwDir),
		"--mount", fmt.Sprintf("type=bind,src=%s,target=/mnt2,ro", roDir),
		testutil.AlpineImage,
		"top",
	).AssertOK()
	base.Cmd("exec", containerName, "sh", "-exc", "echo -n str1 > /mnt1/file1").AssertOK()
	base.Cmd("exec", containerName, "sh", "-exc", "echo -n str2 > /mnt2/file2").AssertFail()

	base.Cmd("run",
		"--rm",
		"--mount", fmt.Sprintf("type=bind,src=%s,target=/mnt1", rwDir),
		testutil.AlpineImage,
		"cat", "/mnt1/file1",
	).AssertOutExactly("str1")

	// check `bind-propagation`
	f := func(allow string) func(stdout string) error {
		return func(stdout string) error {
			lines := strings.Split(strings.TrimSpace(stdout), "\n")
			if len(lines) != 1 {
				return fmt.Errorf("expected 1 lines, got %q", stdout)
			}
			fields := strings.Split(lines[0], " ")
			if len(fields) < 4 {
				return fmt.Errorf("invalid /proc/mounts format %q", stdout)
			}

			options := strings.Split(fields[3], ",")

			found := false
			for _, s := range options {
				if allow == s {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("expected stdout to contain %q, got %+v", allow, options)
			}
			return nil
		}
	}
	base.Cmd("exec", containerName, "grep", "/mnt1", "/proc/mounts").AssertOutWithFunc(f("rw"))
	base.Cmd("exec", containerName, "grep", "/mnt2", "/proc/mounts").AssertOutWithFunc(f("ro"))
}

func TestRunMountBindMode(t *testing.T) {
	if rootlessutil.IsRootless() {
		t.Skip("must be superuser to use mount")
	}
	t.Parallel()
	base := testutil.NewBase(t)

	tmpDir1, err := os.MkdirTemp(t.TempDir(), "rw")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir1)
	tmpDir1Mnt := filepath.Join(tmpDir1, "mnt")
	if err := os.MkdirAll(tmpDir1Mnt, 0700); err != nil {
		t.Fatal(err)
	}

	tmpDir2, err := os.MkdirTemp(t.TempDir(), "ro")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir2)

	if err := mobymount.Mount(tmpDir2, tmpDir1Mnt, "none", "bind,ro"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := mobymount.Unmount(tmpDir1Mnt); err != nil {
			t.Fatal(err)
		}
	}()

	base.Cmd("run",
		"--rm",
		"--mount", fmt.Sprintf("type=bind,bind-nonrecursive,src=%s,target=/mnt1", tmpDir1),
		testutil.AlpineImage,
		"sh", "-euxc", "apk add findmnt -q && findmnt -nR /mnt1",
	).AssertOutWithFunc(func(stdout string) error {
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) != 1 {
			return fmt.Errorf("expected 1 line, got %q", stdout)
		}
		if !strings.HasPrefix(lines[0], "/mnt1") {
			return fmt.Errorf("expected mount /mnt1, got %q", lines[0])
		}
		return nil
	})

	base.Cmd("run",
		"--rm",
		"--mount", fmt.Sprintf("type=bind,bind-nonrecursive=false,src=%s,target=/mnt1", tmpDir1),
		testutil.AlpineImage,
		"sh", "-euxc", "apk add findmnt -q && findmnt -nR /mnt1",
	).AssertOutWithFunc(func(stdout string) error {
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) != 2 {
			return fmt.Errorf("expected 2 line, got %q", stdout)
		}
		if !strings.HasPrefix(lines[0], "/mnt1") {
			return fmt.Errorf("expected mount /mnt1, got %q", lines[0])
		}
		return nil
	})
}

func TestRunVolumeBindMode(t *testing.T) {
	if rootlessutil.IsRootless() {
		t.Skip("must be superuser to use mount")
	}
	testutil.DockerIncompatible(t)
	t.Parallel()
	base := testutil.NewBase(t)

	tmpDir1, err := os.MkdirTemp(t.TempDir(), "rw")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir1)
	tmpDir1Mnt := filepath.Join(tmpDir1, "mnt")
	if err := os.MkdirAll(tmpDir1Mnt, 0700); err != nil {
		t.Fatal(err)
	}

	tmpDir2, err := os.MkdirTemp(t.TempDir(), "ro")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir2)

	if err := mobymount.Mount(tmpDir2, tmpDir1Mnt, "none", "bind,ro"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := mobymount.Unmount(tmpDir1Mnt); err != nil {
			t.Fatal(err)
		}
	}()

	base.Cmd("run",
		"--rm",
		"-v", fmt.Sprintf("%s:/mnt1:bind", tmpDir1),
		testutil.AlpineImage,
		"sh", "-euxc", "apk add findmnt -q && findmnt -nR /mnt1",
	).AssertOutWithFunc(func(stdout string) error {
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) != 1 {
			return fmt.Errorf("expected 1 line, got %q", stdout)
		}
		if !strings.HasPrefix(lines[0], "/mnt1") {
			return fmt.Errorf("expected mount /mnt1, got %q", lines[0])
		}
		return nil
	})

	base.Cmd("run",
		"--rm",
		"-v", fmt.Sprintf("%s:/mnt1:rbind", tmpDir1),
		testutil.AlpineImage,
		"sh", "-euxc", "apk add findmnt -q && findmnt -nR /mnt1",
	).AssertOutWithFunc(func(stdout string) error {
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) != 2 {
			return fmt.Errorf("expected 2 line, got %q", stdout)
		}
		if !strings.HasPrefix(lines[0], "/mnt1") {
			return fmt.Errorf("expected mount /mnt1, got %q", lines[0])
		}
		return nil
	})
}

func TestRunBindMountPropagation(t *testing.T) {
	tID := testutil.Identifier(t)

	if !isRootfsShareableMount() {
		t.Skipf("rootfs doesn't support shared mount, skip test %s", tID)
	}

	t.Parallel()
	base := testutil.NewBase(t)

	testCases := []struct {
		propagation string
		assertFunc  func(containerName, containerNameReplica string)
	}{
		{
			propagation: "rshared",
			assertFunc: func(containerName, containerNameReplica string) {
				// replica can get sub-mounts from original
				base.Cmd("exec", containerNameReplica, "cat", "/mnt1/replica/foo.txt").AssertOutExactly("toreplica")

				// and sub-mounts from replica will be propagated to the original too
				base.Cmd("exec", containerName, "cat", "/mnt1/bar/bar.txt").AssertOutExactly("fromreplica")
			},
		},
		{
			propagation: "rslave",
			assertFunc: func(containerName, containerNameReplica string) {
				// replica can get sub-mounts from original
				base.Cmd("exec", containerNameReplica, "cat", "/mnt1/replica/foo.txt").AssertOutExactly("toreplica")

				// but sub-mounts from replica will not be propagated to the original
				base.Cmd("exec", containerName, "cat", "/mnt1/bar/bar.txt").AssertFail()
			},
		},
		{
			propagation: "rprivate",
			assertFunc: func(containerName, containerNameReplica string) {
				// replica can't get sub-mounts from original
				base.Cmd("exec", containerNameReplica, "cat", "/mnt1/replica/foo.txt").AssertFail()
				// and sub-mounts from replica will not be propagated to the original too
				base.Cmd("exec", containerName, "cat", "/mnt1/bar/bar.txt").AssertFail()
			},
		},
		{
			propagation: "",
			assertFunc: func(containerName, containerNameReplica string) {
				// replica can't get sub-mounts from original
				base.Cmd("exec", containerNameReplica, "cat", "/mnt1/replica/foo.txt").AssertFail()
				// and sub-mounts from replica will not be propagated to the original too
				base.Cmd("exec", containerName, "cat", "/mnt1/bar/bar.txt").AssertFail()
			},
		},
	}

	for _, tc := range testCases {
		propagationName := tc.propagation
		if propagationName == "" {
			propagationName = "default"
		}

		t.Logf("Running test propagation case %s", propagationName)

		rwDir, err := os.MkdirTemp(t.TempDir(), "rw")
		if err != nil {
			t.Fatal(err)
		}

		containerName := tID + "-" + propagationName
		containerNameReplica := containerName + "-replica"

		mountOption := fmt.Sprintf("type=bind,src=%s,target=/mnt1,bind-propagation=%s", rwDir, tc.propagation)
		if tc.propagation == "" {
			mountOption = fmt.Sprintf("type=bind,src=%s,target=/mnt1", rwDir)
		}

		containers := []struct {
			name        string
			mountOption string
		}{
			{
				name:        containerName,
				mountOption: fmt.Sprintf("type=bind,src=%s,target=/mnt1,bind-propagation=rshared", rwDir),
			},
			{
				name:        containerNameReplica,
				mountOption: mountOption,
			},
		}
		for _, c := range containers {
			base.Cmd("run", "-d",
				"--privileged",
				"--name", c.name,
				"--mount", c.mountOption,
				testutil.AlpineImage,
				"top").AssertOK()
			defer base.Cmd("rm", "-f", c.name).Run()
		}

		// mount in the first container
		base.Cmd("exec", containerName, "sh", "-exc", "mkdir /app && mkdir /mnt1/replica && mount --bind /app /mnt1/replica && echo -n toreplica > /app/foo.txt").AssertOK()
		base.Cmd("exec", containerName, "cat", "/mnt1/replica/foo.txt").AssertOutExactly("toreplica")

		// mount in the second container
		base.Cmd("exec", containerNameReplica, "sh", "-exc", "mkdir /bar && mkdir /mnt1/bar").AssertOK()
		base.Cmd("exec", containerNameReplica, "sh", "-exc", "mount --bind /bar /mnt1/bar").AssertOK()

		base.Cmd("exec", containerNameReplica, "sh", "-exc", "echo -n fromreplica > /bar/bar.txt").AssertOK()
		base.Cmd("exec", containerNameReplica, "cat", "/mnt1/bar/bar.txt").AssertOutExactly("fromreplica")

		// call case specific assert function
		tc.assertFunc(containerName, containerNameReplica)

		// umount mount point in the first privileged container
		base.Cmd("exec", containerNameReplica, "sh", "-exc", "umount /mnt1/bar").AssertOK()
		base.Cmd("exec", containerName, "sh", "-exc", "umount /mnt1/replica").AssertOK()
	}
}

// isRootfsShareableMount will check if /tmp or / support shareable mount
func isRootfsShareableMount() bool {
	existFunc := func(mi mount.Info) bool {
		for _, opt := range strings.Split(mi.Optional, " ") {
			if strings.HasPrefix(opt, "shared:") {
				return true
			}
		}
		return false
	}

	mi, err := mount.Lookup("/tmp")
	if err == nil {
		return existFunc(mi)
	}

	mi, err = mount.Lookup("/")
	if err == nil {
		return existFunc(mi)
	}

	return false
}
