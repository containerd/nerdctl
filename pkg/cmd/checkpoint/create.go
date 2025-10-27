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

package checkpoint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/containerd/containerd/api/types/runc/options"
	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/pkg/archive"
	"github.com/containerd/containerd/v2/plugins"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/checkpointutil"
	"github.com/containerd/nerdctl/v2/pkg/idutil/containerwalker"
)

func Create(ctx context.Context, client *containerd.Client, containerID string, checkpointName string, options types.CheckpointCreateOptions) error {
	var container containerd.Container

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple containers found with provided prefix: %s", found.Req)
			}
			container = found.Container
			return nil
		},
	}

	n, err := walker.Walk(ctx, containerID)
	if err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("error creating checkpoint for container: %s, no such container", containerID)
	}

	info, err := container.Info(ctx)
	if err != nil {
		return fmt.Errorf("failed to get info for container %q: %w", containerID, err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to get task for container %q: %w", containerID, err)
	}

	img, err := task.Checkpoint(ctx, withCheckpointOpts(info.Runtime.Name, !options.LeaveRunning))
	if err != nil {
		return err
	}

	defer client.ImageService().Delete(ctx, img.Name())

	cs := client.ContentStore()

	rawIndex, err := content.ReadBlob(ctx, cs, img.Target())
	if err != nil {
		return fmt.Errorf("failed to retrieve checkpoint data: %w", err)
	}

	var index ocispec.Index
	if err := json.Unmarshal(rawIndex, &index); err != nil {
		return fmt.Errorf("failed to decode checkpoint data: %w", err)
	}

	var cpDesc *ocispec.Descriptor
	for _, m := range index.Manifests {
		if m.MediaType == images.MediaTypeContainerd1Checkpoint {
			cpDesc = &m //nolint:gosec
			break
		}
	}
	if cpDesc == nil {
		return errors.New("invalid checkpoint")
	}

	if options.CheckpointDir == "" {
		options.CheckpointDir = filepath.Join(options.GOptions.DataRoot, "checkpoints")
	}
	targetPath, err := checkpointutil.GetCheckpointDir(options.CheckpointDir, checkpointName, container.ID(), true)
	if err != nil {
		return err
	}

	rat, err := cs.ReaderAt(ctx, *cpDesc)
	if err != nil {
		return fmt.Errorf("failed to get checkpoint reader: %w", err)
	}
	defer rat.Close()

	_, err = archive.Apply(ctx, targetPath, content.NewReader(rat))
	if err != nil {
		return fmt.Errorf("failed to read checkpoint reader: %w", err)
	}

	fmt.Fprintf(options.Stdout, "%s\n", checkpointName)

	return nil
}

func withCheckpointOpts(rt string, exit bool) containerd.CheckpointTaskOpts {
	return func(r *containerd.CheckpointTaskInfo) error {

		switch rt {
		case plugins.RuntimeRuncV2:
			if r.Options == nil {
				r.Options = &options.CheckpointOptions{}
			}
			opts, _ := r.Options.(*options.CheckpointOptions)

			opts.Exit = exit
		}
		return nil
	}
}
