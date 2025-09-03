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
	"fmt"

	containerd "github.com/containerd/containerd/v2/client"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/idutil/containerwalker"
)

func Create(ctx context.Context, client *containerd.Client, containerID string, checkpointDir string, options types.CheckpointCreateOptions) error {
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
		return fmt.Errorf("no such container: %s", containerID)
	}

	opts := []containerd.CheckpointOpts{
		containerd.WithCheckpointRuntime,
		containerd.WithCheckpointRW,
		containerd.WithCheckpointTask,
	}
	if !options.LeaveRunning {
		opts = append(opts, containerd.WithCheckpointTaskExit)
	}

	checkpoint, err := container.Checkpoint(ctx, checkpointDir, opts...)
	if err != nil {
		return err
	}

	fmt.Fprintf(options.Stdout, "Checkpoint created: %s\n", checkpoint.Name())
	return nil
}
