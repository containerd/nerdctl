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
	"context"
	"fmt"
	"os"
	"os/exec"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/containerutil"
	"github.com/containerd/nerdctl/v2/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/v2/pkg/tarutil"
)

// Export exports a container's filesystem as a tar archive
func Export(ctx context.Context, client *containerd.Client, containerReq string, options types.ContainerExportOptions) error {
	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			return exportContainer(ctx, client, found.Container, options)
		},
	}

	n, err := walker.Walk(ctx, containerReq)
	if err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("no such container %s", containerReq)
	}
	return nil
}

func exportContainer(ctx context.Context, client *containerd.Client, container containerd.Container, options types.ContainerExportOptions) error {
	// Try to get a running container root first
	root, pid, err := getContainerRoot(ctx, container)
	var cleanup func() error

	if err != nil {
		// Container is not running, try to mount the snapshot
		var conInfo containers.Container
		conInfo, err = container.Info(ctx)
		if err != nil {
			return fmt.Errorf("failed to get container info: %w", err)
		}

		root, cleanup, err = containerutil.MountSnapshotForContainer(ctx, client, conInfo, options.GOptions.Snapshotter)
		if cleanup != nil {
			defer func() {
				if cleanupErr := cleanup(); cleanupErr != nil {
					log.G(ctx).WithError(cleanupErr).Warn("Failed to cleanup mounted snapshot")
				}
			}()
		}

		if err != nil {
			return fmt.Errorf("failed to mount container snapshot: %w", err)
		}
		log.G(ctx).Debugf("Mounted snapshot at %s", root)
		// For stopped containers, set pid to 0 to avoid nsenter
		pid = 0
	} else {
		log.G(ctx).Debugf("Using running container root %s (pid %d)", root, pid)
	}

	// Create tar command to export the rootfs
	return createTarArchive(ctx, root, pid, options)
}

func getContainerRoot(ctx context.Context, container containerd.Container) (string, int, error) {
	task, err := container.Task(ctx, nil)
	if err != nil {
		return "", 0, err
	}

	status, err := task.Status(ctx)
	if err != nil {
		return "", 0, err
	}

	if status.Status != containerd.Running {
		return "", 0, fmt.Errorf("container is not running")
	}

	pid := int(task.Pid())
	return fmt.Sprintf("/proc/%d/root", pid), pid, nil
}

func createTarArchive(ctx context.Context, rootPath string, pid int, options types.ContainerExportOptions) error {
	tarBinary, isGNUTar, tar_err := tarutil.FindTarBinary()
	if tar_err != nil {
		return tar_err
	}
	log.G(ctx).Debugf("Detected tar binary %q (GNU=%v)", tarBinary, isGNUTar)

	// For now, use direct tar access. nsenter may have permission issues in rootless mode.
	tarArgs := []string{"-c", "-f", "-", "-C", rootPath, "."}
	cmd := exec.CommandContext(ctx, tarBinary, tarArgs...)

	log.G(ctx).Debugf("Using tar directly: %s %v", cmd.Path, cmd.Args)

	cmd.Stdout = options.Stdout

	// For running containers (pid > 0), suppress stderr entirely as virtual filesystem
	// errors are expected and not useful to the user. For stopped containers, show stderr
	// as those errors might be legitimate issues.
	if pid > 0 {
		// Running container - suppress all stderr by redirecting to /dev/null
		devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err != nil {
			return fmt.Errorf("failed to open /dev/null: %w", err)
		}
		defer devNull.Close()
		cmd.Stderr = devNull
		log.G(ctx).Debugf("Suppressing stderr for running container export (virtual filesystem errors expected)")
	} else {
		// Stopped container - show stderr as normal
		cmd.Stderr = os.Stderr
	}

	err := cmd.Run()

	// When exporting running containers, tar may fail with exit code 2 due to
	// permission issues with virtual filesystems like /proc and /sys.
	// This is expected behavior and should not cause the export to fail.
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			// Exit code 2 typically indicates "fatal error" but in the context
			// of exporting containers, it's often due to permission denied errors
			// on virtual filesystems which are expected and acceptable.
			if exitError.ExitCode() == 2 && pid > 0 {
				log.G(ctx).Debugf("tar exited with code 2, likely due to permission issues with virtual filesystems (expected for running containers)")
				return nil
			}
		}
		return err
	}

	return nil
}
