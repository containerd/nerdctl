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
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/containerd/nerdctl/pkg/tarutil"
	securejoin "github.com/cyphar/filepath-securejoin"
)

// CopyFiles implements `nerdctl cp`.
// See https://docs.docker.com/engine/reference/commandline/cp/ for the specification.
func CopyFiles(ctx context.Context, client *containerd.Client, container containerd.Container, container2host bool, dst, src string, snapshotter string, followSymlink bool) error {
	tarBinary, isGNUTar, err := tarutil.FindTarBinary()
	if err != nil {
		return err
	}
	log.G(ctx).Debugf("Detected tar binary %q (GNU=%v)", tarBinary, isGNUTar)
	var srcFull, dstFull, root, mountDestination, containerPath string
	var cleanup func()
	task, err := container.Task(ctx, nil)
	if err != nil {
		// FIXME: Rootless does not support copying into/out of stopped/created containers as we need to nsenter into the user namespace of the
		// pid of the running container with --preserve-credentials to preserve uid/gid mapping and copy files into the container.
		if rootlessutil.IsRootless() {
			return errors.New("cannot use cp with stopped containers in rootless mode")
		}
		// if the task is simply not found, we should try to mount the snapshot. any other type of error from Task() is fatal here.
		if !errdefs.IsNotFound(err) {
			return err
		}
		if container2host {
			containerPath = src
		} else {
			containerPath = dst
		}
		// Check if containerPath is in a volume
		root, mountDestination, err = getContainerMountInfo(ctx, container, containerPath, container2host)
		if err != nil {
			return err
		}
		// if containerPath is in a volume and not read-only in case of host2container copy then handle volume paths,
		// else containerPath is not in volume so mount container snapshot for copy
		if root != "" {
			dst, src = handleVolumePaths(container2host, dst, src, mountDestination)
		} else {
			root, cleanup, err = mountSnapshotForContainer(ctx, client, container, snapshotter)
			if cleanup != nil {
				defer cleanup()
			}
			if err != nil {
				return err
			}
		}
	} else {
		status, err := task.Status(ctx)
		if err != nil {
			return err
		}
		if status.Status == containerd.Running {
			root = fmt.Sprintf("/proc/%d/root", task.Pid())
		} else {
			if rootlessutil.IsRootless() {
				return fmt.Errorf("cannot use cp with stopped containers in rootless mode")
			}
			if container2host {
				containerPath = src
			} else {
				containerPath = dst
			}
			root, mountDestination, err = getContainerMountInfo(ctx, container, containerPath, container2host)
			if err != nil {
				return err
			}
			// if containerPath is in a volume and not read-only in case of host2container copy then handle volume paths,
			// else containerPath is not in volume so mount container snapshot for copy
			if root != "" {
				dst, src = handleVolumePaths(container2host, dst, src, mountDestination)
			} else {
				root, cleanup, err = mountSnapshotForContainer(ctx, client, container, snapshotter)
				if cleanup != nil {
					defer cleanup()
				}
				if err != nil {
					return err
				}
			}
		}
	}
	if container2host {
		srcFull, err = securejoin.SecureJoin(root, src)
		dstFull = dst
	} else {
		srcFull = src
		dstFull, err = securejoin.SecureJoin(root, dst)
	}
	if err != nil {
		return err
	}
	var (
		srcIsDir       bool
		dstExists      bool
		dstExistsAsDir bool
		st             fs.FileInfo
	)
	st, err = os.Stat(srcFull)
	if err != nil {
		return err
	}
	srcIsDir = st.IsDir()

	// dst may not exist yet, so err is negligible
	if st, err := os.Stat(dstFull); err == nil {
		dstExists = true
		dstExistsAsDir = st.IsDir()
	}
	dstEndsWithSep := strings.HasSuffix(dst, string(os.PathSeparator))
	srcEndsWithSlashDot := strings.HasSuffix(src, string(os.PathSeparator)+".")
	if !srcIsDir && dstEndsWithSep && !dstExistsAsDir {
		// The error is specified in https://docs.docker.com/engine/reference/commandline/cp/
		// See the `DEST_PATH does not exist and ends with /` case.
		return fmt.Errorf("the destination directory must exists: %w", err)
	}
	if !srcIsDir && srcEndsWithSlashDot {
		return fmt.Errorf("the source is not a directory")
	}
	if srcIsDir && dstExists && !dstExistsAsDir {
		return fmt.Errorf("cannot copy a directory to a file")
	}
	if srcIsDir && !dstExists {
		if err := os.MkdirAll(dstFull, 0o755); err != nil {
			return err
		}
	}

	var tarCDir, tarCArg string
	if srcIsDir {
		if !dstExists || srcEndsWithSlashDot {
			// the content of the source directory is copied into this directory
			tarCDir = srcFull
			tarCArg = "."
		} else {
			// the source directory is copied into this directory
			tarCDir = filepath.Dir(srcFull)
			tarCArg = filepath.Base(srcFull)
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
		if followSymlink {
			cp = append(cp, "-L")
		}
		if dstEndsWithSep || dstExistsAsDir {
			tarCArg = filepath.Base(srcFull)
		} else {
			// Handle `nerdctl cp /path/to/file some-container:/path/to/file-with-another-name`
			tarCArg = filepath.Base(dstFull)
		}
		cp = append(cp, srcFull, filepath.Join(td, tarCArg))
		cpCmd := exec.CommandContext(ctx, cp[0], cp[1:]...)
		log.G(ctx).Debugf("executing %v", cpCmd.Args)
		if out, err := cpCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to execute %v: %w (out=%q)", cpCmd.Args, err, string(out))
		}
	}
	tarC := []string{tarBinary}
	if followSymlink {
		tarC = append(tarC, "-h")
	}
	tarC = append(tarC, "-c", "-f", "-", tarCArg)

	tarXDir := dstFull
	if !srcIsDir && !dstEndsWithSep && !dstExistsAsDir {
		tarXDir = filepath.Dir(dstFull)
	}
	tarX := []string{tarBinary, "-x"}
	if container2host && isGNUTar {
		tarX = append(tarX, "--no-same-owner")
	}
	tarX = append(tarX, "-f", "-")

	if rootlessutil.IsRootless() {
		nsenter := []string{"nsenter", "-t", strconv.Itoa(int(task.Pid())), "-U", "--preserve-credentials", "--"}
		if container2host {
			tarC = append(nsenter, tarC...)
		} else {
			tarX = append(nsenter, tarX...)
		}
	}

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
	tarXCmd.Stderr = os.Stderr

	log.G(ctx).Debugf("executing %v in %q", tarCCmd.Args, tarCCmd.Dir)
	if err := tarCCmd.Start(); err != nil {
		return fmt.Errorf("failed to execute %v: %w", tarCCmd.Args, err)
	}
	log.G(ctx).Debugf("executing %v in %q", tarXCmd.Args, tarXCmd.Dir)
	if err := tarXCmd.Start(); err != nil {
		return fmt.Errorf("failed to execute %v: %w", tarXCmd.Args, err)
	}
	if err := tarCCmd.Wait(); err != nil {
		return fmt.Errorf("failed to wait %v: %w", tarCCmd.Args, err)
	}
	if err := tarXCmd.Wait(); err != nil {
		return fmt.Errorf("failed to wait %v: %w", tarXCmd.Args, err)
	}
	return nil
}

func mountSnapshotForContainer(ctx context.Context, client *containerd.Client, container containerd.Container, snapshotter string) (string, func(), error) {
	cinfo, err := container.Info(ctx)
	if err != nil {
		return "", nil, err
	}
	snapKey := cinfo.SnapshotKey
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
		return "", nil, fmt.Errorf("failed to mount snapshot with error %s", err.Error())
	}
	cleanup := func() {
		err = mount.Unmount(tempDir, 0)
		if err != nil {
			log.G(ctx).Warnf("failed to unmount %s with error %s", tempDir, err.Error())
			return
		}
		os.RemoveAll(tempDir)
	}
	return tempDir, cleanup, nil
}

func getContainerMountInfo(ctx context.Context, con containerd.Container, containerPath string, container2host bool) (string, string, error) {
	filePath := filepath.Clean(containerPath)
	spec, err := con.Spec(ctx)
	if err != nil {
		return "", "", err
	}
	// read-only applies only while copying into container from host
	if !container2host && spec.Root.Readonly {
		return "", "", fmt.Errorf("container rootfs: %s is marked read-only", spec.Root.Path)
	}

	for _, mount := range spec.Mounts {
		if isSelfOrAscendant(filePath, mount.Destination) {
			// read-only applies only while copying into container from host
			if !container2host {
				for _, option := range mount.Options {
					if option == "ro" {
						return "", "", fmt.Errorf("mount point %s is marked read-only", filePath)
					}
				}
			}
			return mount.Source, mount.Destination, nil
		}
	}
	return "", "", nil
}

func isSelfOrAscendant(filePath, potentialAncestor string) bool {
	if filePath == "/" || filePath == "" || potentialAncestor == "" {
		return false
	}
	filePath = filepath.Clean(filePath)
	potentialAncestor = filepath.Clean(potentialAncestor)
	if filePath == potentialAncestor {
		return true
	}
	return isSelfOrAscendant(path.Dir(filePath), potentialAncestor)
}

// When the path is in volume remove directory that volume is mounted on from the path
func handleVolumePaths(container2host bool, dst string, src string, mountDestination string) (string, string) {
	if container2host {
		return dst, strings.TrimPrefix(filepath.Clean(src), mountDestination)
	}
	return strings.TrimPrefix(filepath.Clean(dst), mountDestination), src
}
