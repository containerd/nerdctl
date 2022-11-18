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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/containerd/nerdctl/pkg/tarutil"
	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newCpCommand() *cobra.Command {

	shortHelp := "Copy files/folders between a running container and the local filesystem."

	longHelp := shortHelp + `
This command requires 'tar' to be installed on the host (not in the container).
Using GNU tar is recommended.
The path of the 'tar' binary can be specified with an environment variable '$TAR'.

WARNING: 'nerdctl cp' is designed only for use with trusted, cooperating containers.
Using 'nerdctl cp' with untrusted or malicious containers is unsupported and may not provide protection against unexpected behavior.
`

	usage := `cp [OPTIONS] CONTAINER:SRC_PATH DEST_PATH|-
  nerdctl cp [OPTIONS] SRC_PATH|- CONTAINER:DEST_PATH`
	var cpCommand = &cobra.Command{
		Use:               usage,
		Args:              cobra.ExactArgs(2),
		Short:             shortHelp,
		Long:              longHelp,
		RunE:              cpAction,
		ValidArgsFunction: cpShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}

	cpCommand.Flags().BoolP("follow-link", "L", false, "Always follow symbolic link in SRC_PATH.")

	return cpCommand
}

func cpAction(cmd *cobra.Command, args []string) error {
	srcSpec, err := parseCpFileSpec(args[0])
	if err != nil {
		return err
	}

	destSpec, err := parseCpFileSpec(args[1])
	if err != nil {
		return err
	}

	flagL, err := cmd.Flags().GetBool("follow-link")
	if err != nil {
		return err
	}

	if (srcSpec.Container != nil && destSpec.Container != nil) || (len(srcSpec.Path) == 0 && len(destSpec.Path) == 0) {
		return fmt.Errorf("one of src or dest must be a local file specification")
	}
	if srcSpec.Container == nil && destSpec.Container == nil {
		return fmt.Errorf("one of src or dest must be a container file specification")
	}
	if srcSpec.Path == "-" {
		return fmt.Errorf("support for reading a tar archive from stdin is not implemented yet")
	}
	if destSpec.Path == "-" {
		return fmt.Errorf("support for writing a tar archive to stdout is not implemented yet")
	}

	container2host := srcSpec.Container != nil
	var container string
	if container2host {
		container = *srcSpec.Container
	} else {
		container = *destSpec.Container
	}
	ctx := cmd.Context()

	// cp works in the host namespace (for inspecting file permissions), so we can't directly use the Go client.
	selfExe, inspectArgs := globalFlags(cmd)
	inspectArgs = append(inspectArgs, "container", "inspect", "--mode=native", "--format={{json .Process}}", container)
	inspectCmd := exec.CommandContext(ctx, selfExe, inspectArgs...)
	inspectCmd.Stderr = os.Stderr
	inspectOut, err := inspectCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to execute %v: %w", inspectCmd.Args, err)
	}
	var proc native.Process
	if err := json.Unmarshal(inspectOut, &proc); err != nil {
		return err
	}
	if proc.Status.Status != containerd.Running {
		return fmt.Errorf("expected container status %v, got %v", containerd.Running, proc.Status.Status)
	}
	if proc.Pid <= 0 {
		return fmt.Errorf("got non-positive PID %v", proc.Pid)
	}
	return kopy(ctx, container2host, proc.Pid, destSpec.Path, srcSpec.Path, flagL)
}

// kopy implements `nerdctl cp`.
//
// See https://docs.docker.com/engine/reference/commandline/cp/ for the specification.
func kopy(ctx context.Context, container2host bool, pid int, dst, src string, followSymlink bool) error {
	tarBinary, isGNUTar, err := tarutil.FindTarBinary()
	if err != nil {
		return err
	}
	logrus.Debugf("Detected tar binary %q (GNU=%v)", tarBinary, isGNUTar)
	var srcFull, dstFull string
	root := fmt.Sprintf("/proc/%d/root", pid)
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
	)
	if st, err := os.Stat(srcFull); err != nil {
		return err
	} else {
		srcIsDir = st.IsDir()
	}
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
		nsenter := []string{"nsenter", "-t", strconv.Itoa(pid), "-U", "--preserve-credentials", "--"}
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
