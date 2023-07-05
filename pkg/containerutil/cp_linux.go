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
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/containerd/nerdctl/pkg/tarutil"
	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/sirupsen/logrus"
)

// CopyFiles implements `nerdctl cp`.
//
// See https://docs.docker.com/engine/reference/commandline/cp/ for the specification.
func CopyFiles(ctx context.Context, client *containerd.Client, container containerd.Container, container2host bool, dst, src string, snapshotter string, followSymlink bool) error {
	tarBinary, isGNUTar, err := tarutil.FindTarBinary()
	if err != nil {
		return err
	}
	logrus.Debugf("Detected tar binary %q (GNU=%v)", tarBinary, isGNUTar)
	var srcFull, dstFull string
	task, err := container.Task(ctx, nil)
	if err != nil {
		return err
	}
	status, err := task.Status(ctx)
	if err != nil {
		return err
	}
	var root string
	if status.Status == containerd.Running {
		root = fmt.Sprintf("/proc/%d/root", task.Pid())
	} else {
		cinfo, err := container.Info(ctx)
		if err != nil {
			return err
		}
		snapKey := cinfo.SnapshotKey
		resp, err := client.SnapshotService(snapshotter).Mounts(ctx, snapKey)
		root, err = os.MkdirTemp("", "nerdctl-cp-")
		defer os.RemoveAll(root)
		err = mount.All(resp, root)
		if err != nil {
			return err
		}
		defer mount.Unmount(root, 0)
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
		if err := os.MkdirAll(dstFull, 0755); err != nil {
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
		logrus.Debugf("executing %v", cpCmd.Args)
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

	logrus.Debugf("executing %v in %q", tarCCmd.Args, tarCCmd.Dir)
	if err := tarCCmd.Start(); err != nil {
		return fmt.Errorf("failed to execute %v: %w", tarCCmd.Args, err)
	}
	logrus.Debugf("executing %v in %q", tarXCmd.Args, tarXCmd.Dir)
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
