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
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/tarutil"
)

// See https://docs.docker.com/engine/reference/commandline/cp/ for the specification.

var (
	// Generic and system errors
	ErrFilesystem             = errors.New("filesystem error") // lstat hard errors, etc
	ErrContainerVanished      = errors.New("the container you are trying to copy to/from has been deleted")
	ErrRootlessCannotCp       = errors.New("cannot use cp with stopped containers in rootless mode") // rootless cp with a stopped container
	ErrFailedMountingSnapshot = errors.New("failed mounting snapshot")                               // failure to mount a stopped container snapshot

	// CP specific errors
	ErrTargetIsReadOnly           = errors.New("cannot copy into read-only location")                            // ...
	ErrSourceIsNotADir            = errors.New("source is not a directory")                                      // cp SOMEFILE/ foo:/
	ErrDestinationIsNotADir       = errors.New("destination is not a directory")                                 // * cp ./ foo:/etc/issue/bah
	ErrSourceDoesNotExist         = errors.New("source does not exist")                                          // cp NONEXISTENT foo:/
	ErrDestinationParentMustExist = errors.New("destination parent does not exist")                              // nerdctl cp VALID_PATH foo:/NONEXISTENT/NONEXISTENT
	ErrDestinationDirMustExist    = errors.New("the destination directory must exist to be able to copy a file") // * cp SOMEFILE foo:/NONEXISTENT/
	ErrCannotCopyDirToFile        = errors.New("cannot copy a directory to a file")                              // cp SOMEDIR foo:/etc/issue
)

// getRoot will tentatively return the root of the container on the host (/proc/pid/root), along with the pid,
// (eg: doable when the container is running)
func getRoot(ctx context.Context, container containerd.Container) (string, int, error) {
	task, err := container.Task(ctx, nil)
	if err != nil {
		return "", 0, err
	}

	status, err := task.Status(ctx)
	if err != nil {
		return "", 0, err
	}

	if status.Status != containerd.Running {
		return "", 0, nil
	}
	pid := int(task.Pid())

	return fmt.Sprintf("/proc/%d/root", pid), pid, nil
}

// CopyFiles implements `nerdctl cp`
// It currently depends on the following assumptions:
// - linux only
// - tar binary exists on the system
// - nsenter binary exists on the system
// - if rootless, the container is running (aka: /proc/pid/root)
func CopyFiles(ctx context.Context, client *containerd.Client, container containerd.Container, options types.ContainerCpOptions) (err error) {
	// We do rely on the tar binary as a shortcut - could also be replaced by archive/tar, though that would mean
	// we need to replace nsenter calls with re-exec
	tarBinary, isGNUTar, err := tarutil.FindTarBinary()
	if err != nil {
		return err
	}

	log.G(ctx).Debugf("Detected tar binary %q (GNU=%v)", tarBinary, isGNUTar)

	// This can happen if the container being passed has been deleted since in a racy way
	conSpec, err := container.Spec(ctx)
	if err != nil {
		return errors.Join(ErrContainerVanished, err)
	}

	// Try to get a running container root
	root, pid, err := getRoot(ctx, container)
	// If the task is "not found" (for example, if the container stopped), we will try to mount the snapshot
	// Any other type of error from Task() is fatal here.
	if err != nil && !errdefs.IsNotFound(err) {
		return errors.Join(ErrContainerVanished, err)
	}

	log.G(ctx).Debugf("We have root %s and pid %d", root, pid)

	// If we have no root:
	// - bail out for rootless
	// - mount the snapshot for rootful
	if root == "" {
		// FIXME: Rootless does not support copying into/out of stopped/created containers as we need to nsenter into
		// the user namespace of the pid of the running container with --preserve-credentials to preserve uid/gid
		// mapping and copy files into the container.
		if rootlessutil.IsRootless() {
			return ErrRootlessCannotCp
		}

		// See similar situation above. This may happen if we are racing against container deletion
		var conInfo containers.Container
		conInfo, err = container.Info(ctx)
		if err != nil {
			return errors.Join(ErrContainerVanished, err)
		}

		var cleanup func() error
		root, cleanup, err = MountSnapshotForContainer(ctx, client, conInfo, options.GOptions.Snapshotter)
		if cleanup != nil {
			defer func() {
				err = errors.Join(err, cleanup())
			}()
		}

		if err != nil {
			return errors.Join(ErrFailedMountingSnapshot, err)
		}

		log.G(ctx).Debugf("Got new root %s", root)
	}

	var sourceSpec, destinationSpec *pathSpecifier
	var sourceErr, destErr error
	if options.Container2Host {
		sourceSpec, sourceErr = getPathSpecFromContainer(options.SrcPath, conSpec, root)
		destinationSpec, destErr = getPathSpecFromHost(options.DestPath)
	} else {
		sourceSpec, sourceErr = getPathSpecFromHost(options.SrcPath)
		destinationSpec, destErr = getPathSpecFromContainer(options.DestPath, conSpec, root)
	}

	if destErr != nil {
		if errors.Is(destErr, errDoesNotExist) {
			return ErrDestinationParentMustExist
		} else if errors.Is(destErr, errIsNotADir) {
			return ErrDestinationIsNotADir
		}

		return errors.Join(ErrFilesystem, destErr)
	}

	if sourceErr != nil {
		if errors.Is(sourceErr, errDoesNotExist) {
			return ErrSourceDoesNotExist
		} else if errors.Is(sourceErr, errIsNotADir) {
			return ErrSourceIsNotADir
		}

		return errors.Join(ErrFilesystem, sourceErr)
	}

	// Now, resolve cp shenanigans
	// First, cannot copy a non-existent resource
	if !sourceSpec.exists {
		return ErrSourceDoesNotExist
	}

	// Second, cannot copy into a readonly destination
	if destinationSpec.readOnly {
		return ErrTargetIsReadOnly
	}

	// Cannot copy a dir into a file
	if sourceSpec.isADir && destinationSpec.exists && !destinationSpec.isADir {
		return ErrCannotCopyDirToFile
	}

	// A file cannot be copied inside a non-existent directory with a trailing slash, or slash+dot
	if !sourceSpec.isADir && !destinationSpec.exists && (destinationSpec.endsWithSeparator || destinationSpec.endsWithSeparatorDot) {
		return ErrDestinationDirMustExist
	}

	// XXX FIXME: this seems wrong. What about ownership? We could be doing that inside a container
	if !destinationSpec.exists {
		if err = os.Mkdir(destinationSpec.resolvedPath, 0o755); err != nil {
			return errors.Join(ErrFilesystem, err)
		}
	}

	var tarCDir, tarCArg string
	if sourceSpec.isADir {
		if !destinationSpec.exists || sourceSpec.endsWithSeparatorDot {
			// the content of the source directory is copied into this directory
			tarCDir = sourceSpec.resolvedPath
			tarCArg = "."
		} else {
			// the source directory is copied into this directory
			tarCDir = filepath.Dir(sourceSpec.resolvedPath)
			tarCArg = filepath.Base(sourceSpec.resolvedPath)
		}
	} else {
		// Prepare a single-file directory to create an archive of the source file
		td, err := os.MkdirTemp("", "nerdctl-cp")
		if err != nil {
			return err
		}
		defer os.RemoveAll(td)
		tarCDir = td
		cp := []string{"cp", "-a"}
		if options.FollowSymLink {
			cp = append(cp, "-L")
		}
		if destinationSpec.endsWithSeparator || (destinationSpec.exists && destinationSpec.isADir) {
			tarCArg = filepath.Base(sourceSpec.resolvedPath)
		} else {
			// Handle `nerdctl cp /path/to/file some-container:/path/to/file-with-another-name`
			tarCArg = filepath.Base(destinationSpec.resolvedPath)
		}
		cp = append(cp, sourceSpec.resolvedPath, filepath.Join(td, tarCArg))
		cpCmd := exec.CommandContext(ctx, cp[0], cp[1:]...)
		log.G(ctx).Debugf("executing %v", cpCmd.Args)
		if out, err := cpCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to execute %v: %w (out=%q)", cpCmd.Args, err, string(out))
		}
	}
	tarC := []string{tarBinary}
	if options.FollowSymLink {
		tarC = append(tarC, "-h")
	}
	tarC = append(tarC, "-c", "-f", "-", tarCArg)

	tarXDir := destinationSpec.resolvedPath
	if !sourceSpec.isADir && !destinationSpec.endsWithSeparator && !(destinationSpec.exists && destinationSpec.isADir) {
		tarXDir = filepath.Dir(destinationSpec.resolvedPath)
	}
	tarX := []string{tarBinary, "-x"}
	if options.Container2Host && isGNUTar {
		tarX = append(tarX, "--no-same-owner")
	}
	tarX = append(tarX, "-f", "-")

	if rootlessutil.IsRootless() {
		nsenter := []string{"nsenter", "-t", strconv.Itoa(pid), "-U", "--preserve-credentials", "--"}
		if options.Container2Host {
			tarC = append(nsenter, tarC...)
		} else {
			tarX = append(nsenter, tarX...)
		}
	}

	// FIXME: moving to archive/tar should allow better error management than this
	// WARNING: some of our testing on stderr might not be portable across different versions of tar
	// In these cases (readonly target), we will just get the straight tar output instead
	tarCCmd := exec.CommandContext(ctx, tarC[0], tarC[1:]...)
	tarCCmd.Dir = tarCDir
	tarCCmd.Stdin = nil
	tarCCmd.Stderr = os.Stderr

	tarXCmd := exec.CommandContext(ctx, tarX[0], tarX[1:]...)
	tarXCmd.Dir = tarXDir
	tarXCmd.Stdin, err = tarCCmd.StdoutPipe()
	if err != nil {
		return err
	}
	tarXCmd.Stdout = os.Stderr
	var tarErr bytes.Buffer
	tarXCmd.Stderr = &tarErr

	log.G(ctx).Debugf("executing %v in %q", tarCCmd.Args, tarCCmd.Dir)
	if err := tarCCmd.Start(); err != nil {
		return errors.Join(fmt.Errorf("failed to execute %v", tarCCmd.Args), err)
	}

	log.G(ctx).Debugf("executing %v in %q", tarXCmd.Args, tarXCmd.Dir)
	if err := tarXCmd.Start(); err != nil {
		if strings.Contains(err.Error(), "permission denied") {
			return ErrTargetIsReadOnly
		}

		// Other errors, just put them back on stderr
		_, fpErr := fmt.Fprint(os.Stderr, tarErr.String())
		if fpErr != nil {
			return errors.Join(fpErr, err)
		}

		return errors.Join(fmt.Errorf("failed to execute %v", tarXCmd.Args), err)
	}

	if err := tarCCmd.Wait(); err != nil {
		return fmt.Errorf("failed to wait %v: %w", tarCCmd.Args, err)
	}

	if err := tarXCmd.Wait(); err != nil {
		if strings.Contains(tarErr.String(), "Read-only file system") {
			return ErrTargetIsReadOnly
		}

		// Other errors, just put them back on stderr
		_, fpErr := fmt.Fprint(os.Stderr, tarErr.String())
		if fpErr != nil {
			return errors.Join(fpErr, err)
		}

		return errors.Join(fmt.Errorf("failed to wait %v", tarXCmd.Args), err)
	}

	return nil
}

func MountSnapshotForContainer(ctx context.Context, client *containerd.Client, conInfo containers.Container, snapshotter string) (string, func() error, error) {
	snapKey := conInfo.SnapshotKey
	resp, err := client.SnapshotService(snapshotter).Mounts(ctx, snapKey)
	if err != nil {
		return "", nil, err
	}

	tempDir, err := os.MkdirTemp("", "nerdctl-cp-")
	if err != nil {
		return "", nil, err
	}

	err = mount.All(resp, tempDir)
	if err != nil {
		return "", nil, err
	}

	cleanup := func() error {
		err = mount.Unmount(tempDir, 0)
		if err != nil {
			return err
		}
		return os.RemoveAll(tempDir)
	}

	return tempDir, cleanup, nil
}
