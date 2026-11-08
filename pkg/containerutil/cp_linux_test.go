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

package containerutil

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/opencontainers/runtime-spec/specs-go"
	"gotest.tools/v3/assert"

	"github.com/containerd/containerd/v2/pkg/oci"

	"github.com/containerd/nerdctl/v2/pkg/tarutil"
)

// tempDir returns a new temporary directory with symlinks resolved, the way the kernel reports open files.
func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	assert.NilError(t, err)
	return dir
}

// containerRoot returns a directory standing in for /proc/<pid>/root, with a volume mounted at /vol.
func containerRoot(t *testing.T) (string, string, *oci.Spec) {
	t.Helper()
	root := tempDir(t)
	volume := tempDir(t)
	assert.NilError(t, os.MkdirAll(filepath.Join(root, "data", "x"), 0o755))
	assert.NilError(t, os.MkdirAll(filepath.Join(root, "vol"), 0o755))
	assert.NilError(t, os.MkdirAll(filepath.Join(volume, "sub"), 0o755))
	return root, volume, &oci.Spec{
		Root:   &specs.Root{Path: "rootfs"},
		Mounts: []specs.Mount{{Destination: "/vol", Source: volume, Options: []string{"rbind"}}},
	}
}

// swapForSymlink replaces the directory at path with a symlink to target, as a process in the container can
// between the time cp resolves the destination and the time it writes to it.
func swapForSymlink(t *testing.T, path, target string) {
	t.Helper()
	assert.NilError(t, os.Rename(path, path+".orig"))
	assert.NilError(t, os.Symlink(target, path))
}

func handlePath(t *testing.T, f *os.File) string {
	t.Helper()
	p, err := os.Readlink("/proc/self/fd/" + strconv.Itoa(int(f.Fd())))
	assert.NilError(t, err)
	return p
}

func TestGetPathSpecFromContainerRecordsMount(t *testing.T) {
	root, volume, conSpec := containerRoot(t)

	spec, err := getPathSpecFromContainer("/data/newfile", conSpec, root)
	assert.NilError(t, err)
	assert.Equal(t, spec.mountRoot, root)
	assert.Equal(t, spec.relativePath, filepath.Join("data", "newfile"))

	spec, err = getPathSpecFromContainer("/vol/sub", conSpec, root)
	assert.NilError(t, err)
	assert.Equal(t, spec.mountRoot, volume)
	assert.Equal(t, spec.relativePath, "sub")
}

func TestOpenExtractDir(t *testing.T) {
	root, volume, conSpec := containerRoot(t)

	// a file copied to a new name: the destination is created, as before, and tar extracts into the parent
	spec, err := getPathSpecFromContainer("/data/x/newname", conSpec, root)
	assert.NilError(t, err)
	dir, err := openExtractDir(spec, false)
	assert.NilError(t, err)
	assert.Equal(t, handlePath(t, dir), filepath.Join(root, "data", "x"))
	assert.NilError(t, dir.Close())
	st, err := os.Lstat(filepath.Join(root, "data", "x", "newname"))
	assert.NilError(t, err)
	assert.Assert(t, st.IsDir())

	// a directory copied to a new name in a volume: the destination is created and tar extracts into it
	spec, err = getPathSpecFromContainer("/vol/newdir", conSpec, root)
	assert.NilError(t, err)
	dir, err = openExtractDir(spec, true)
	assert.NilError(t, err)
	assert.Equal(t, handlePath(t, dir), filepath.Join(volume, "newdir"))
	assert.NilError(t, dir.Close())
}

func TestOpenExtractDirFileBindMount(t *testing.T) {
	root := tempDir(t)
	hostDir := tempDir(t)
	hosts := filepath.Join(hostDir, "hosts")
	assert.NilError(t, os.MkdirAll(filepath.Join(root, "etc"), 0o755))
	assert.NilError(t, os.WriteFile(filepath.Join(root, "etc", "hosts"), nil, 0o644))
	assert.NilError(t, os.WriteFile(hosts, nil, 0o644))
	conSpec := &oci.Spec{
		Root:   &specs.Root{Path: "rootfs"},
		Mounts: []specs.Mount{{Destination: "/etc/hosts", Source: hosts, Options: []string{"rbind"}}},
	}

	spec, err := getPathSpecFromContainer("/etc/hosts", conSpec, root)
	assert.NilError(t, err)
	dir, err := openExtractDir(spec, false)
	assert.NilError(t, err)
	defer dir.Close()
	assert.Equal(t, handlePath(t, dir), hostDir)
}

func TestOpenExtractDirRejectsNonPathSource(t *testing.T) {
	_, err := openExtractDir(&pathSpecifier{mountRoot: "tmpfs", relativePath: "x"}, true)
	assert.Assert(t, errors.Is(err, ErrFilesystem))
	assert.ErrorContains(t, err, "non-path source")
}

// TestOpenExtractDirSwapped swaps a component of the destination for a symlink leaving the container after it has
// been resolved, both for a destination that exists and for one that has to be created.
func TestOpenExtractDirSwapped(t *testing.T) {
	for _, tc := range []struct {
		name     string
		relative bool
		dest     string
		intoDest bool
	}{
		{"absolute symlink, existing destination", false, "/data/x", true},
		{"absolute symlink, created destination", false, "/data/x/new", true},
		{"relative symlink, existing destination", true, "/data/x", true},
		{"relative symlink, created destination", true, "/data/x/new", true},
		{"absolute symlink, file to new name", false, "/data/x/newname", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, _, conSpec := containerRoot(t)
			outside := tempDir(t)
			spec, err := getPathSpecFromContainer(tc.dest, conSpec, root)
			assert.NilError(t, err)

			target := outside
			if tc.relative {
				target, err = filepath.Rel(filepath.Join(root, "data"), outside)
				assert.NilError(t, err)
			}
			swapForSymlink(t, filepath.Join(root, "data", "x"), target)

			dir, err := openExtractDir(spec, tc.intoDest)
			if err == nil {
				p := handlePath(t, dir)
				dir.Close()
				t.Fatalf("opened %s outside of the container", p)
			}
			assert.Assert(t, errors.Is(err, ErrFilesystem), err)
			entries, err := os.ReadDir(outside)
			assert.NilError(t, err)
			assert.Equal(t, len(entries), 0, "created something outside of the container")
		})
	}
}

func TestTarExtractCmdUnpinned(t *testing.T) {
	cmd := tarExtractCmd(context.Background(), []string{"tar", "-x", "--no-same-owner"}, "/some/dir", nil)
	assert.DeepEqual(t, cmd.Args, []string{"tar", "-x", "--no-same-owner", "-C", "/some/dir", "-f", "-"})
	assert.Assert(t, cmd.ExtraFiles == nil)
}

// TestTarExtractCmd runs the tar command CopyFiles uses for a copy into the container: after the handle is opened,
// swapping the directory for a symlink does not redirect the extraction.
func TestTarExtractCmd(t *testing.T) {
	tarBinary, _, err := tarutil.FindTarBinary()
	if err != nil {
		t.Skipf("no tar binary: %v", err)
	}

	src := tempDir(t)
	outside := tempDir(t)
	root, _, conSpec := containerRoot(t)
	assert.NilError(t, os.WriteFile(filepath.Join(src, "payload"), []byte("x"), 0o644))

	spec, err := getPathSpecFromContainer("/data/x", conSpec, root)
	assert.NilError(t, err)
	dir, err := openExtractDir(spec, true)
	assert.NilError(t, err)
	defer dir.Close()

	swapForSymlink(t, filepath.Join(root, "data", "x"), outside)

	archive := exec.Command(tarBinary, "-C", src, "-c", "-f", "-", "payload")
	extract := tarExtractCmd(context.Background(), []string{tarBinary, "-x"}, filepath.Join(root, "data", "x"), dir)
	extract.Stdin, err = archive.StdoutPipe()
	assert.NilError(t, err)
	assert.NilError(t, archive.Start())
	out, err := extract.CombinedOutput()
	archiveErr := archive.Wait()
	assert.NilError(t, err, string(out))
	assert.NilError(t, archiveErr)

	_, err = os.Lstat(filepath.Join(outside, "payload"))
	assert.Assert(t, os.IsNotExist(err), "extraction followed the swapped symlink out of the container")
	_, err = os.Lstat(filepath.Join(root, "data", "x.orig", "payload"))
	assert.NilError(t, err, "extraction did not land in the pinned directory")
}
