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

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/containerd/v2/pkg/archive"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/idutil/containerwalker"
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
	// Get container info to access the snapshot
	conInfo, err := container.Info(ctx)
	if err != nil {
		return fmt.Errorf("failed to get container info: %w", err)
	}

	// Use the container's snapshot service to get mounts
	// This works for both running and stopped containers
	sn := client.SnapshotService(conInfo.Snapshotter)
	mounts, err := sn.Mounts(ctx, container.ID())
	if err != nil {
		return fmt.Errorf("failed to get container mounts: %w", err)
	}

	// Create a temporary directory to mount the snapshot
	tempDir, err := os.MkdirTemp("", "nerdctl-export-")
	if err != nil {
		return fmt.Errorf("failed to create temporary mount directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Mount the container's filesystem
	err = mount.All(mounts, tempDir)
	if err != nil {
		return fmt.Errorf("failed to mount container snapshot: %w", err)
	}
	defer func() {
		if unmountErr := mount.Unmount(tempDir, 0); unmountErr != nil {
			log.G(ctx).WithError(unmountErr).Warn("Failed to unmount snapshot")
		}
	}()

	log.G(ctx).Debugf("Mounted container snapshot at %s", tempDir)

	// Create tar archive using WriteDiff
	return createTarArchiveWithWriteDiff(ctx, tempDir, options)
}

func createTarArchiveWithWriteDiff(ctx context.Context, rootPath string, options types.ContainerExportOptions) error {
	// Create a temporary empty directory to use as the "before" state for WriteDiff
	emptyDir, err := os.MkdirTemp("", "nerdctl-export-empty-")
	if err != nil {
		return fmt.Errorf("failed to create temporary empty directory: %w", err)
	}
	defer os.RemoveAll(emptyDir)

	// Debug logging
	log.G(ctx).Debugf("Using WriteDiff to export container filesystem from %s", rootPath)
	log.G(ctx).Debugf("Empty directory: %s", emptyDir)
	log.G(ctx).Debugf("Output writer type: %T", options.Stdout)

	// Check if the rootPath directory exists and has contents
	if entries, err := os.ReadDir(rootPath); err != nil {
		log.G(ctx).Debugf("Failed to read rootPath directory %s: %v", rootPath, err)
	} else {
		log.G(ctx).Debugf("RootPath %s contains %d entries", rootPath, len(entries))
		for i, entry := range entries {
			if i < 10 { // Only log first 10 entries to avoid spam
				log.G(ctx).Debugf("  - %s (dir: %v)", entry.Name(), entry.IsDir())
			}
		}
		if len(entries) > 10 {
			log.G(ctx).Debugf("  ... and %d more entries", len(entries)-10)
		}
	}

	// Double check that emptyDir is empty
	if entries, err := os.ReadDir(emptyDir); err != nil {
		log.G(ctx).Debugf("Failed to read emptyDir directory %s: %v", emptyDir, err)
	} else {
		log.G(ctx).Debugf("EmptyDir %s contains %d entries", emptyDir, len(entries))
		for i, entry := range entries {
			if i < 10 { // Only log first 10 entries to avoid spam
				log.G(ctx).Debugf("  - %s (dir: %v)", entry.Name(), entry.IsDir())
			}
		}
		if len(entries) > 10 {
			log.G(ctx).Debugf("  ... and %d more entries", len(entries)-10)
		}
	}

	// Use WriteDiff to create a tar stream comparing the container rootfs (rootPath)
	// with an empty directory (emptyDir). This produces a complete export of the container.
	err = archive.WriteDiff(ctx, options.Stdout, emptyDir, rootPath)
	if err != nil {
		return fmt.Errorf("failed to write tar diff: %w", err)
	}

	log.G(ctx).Debugf("WriteDiff completed successfully")

	return nil
}
