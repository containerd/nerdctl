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

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/hashicorp/go-multierror"
)

// Wait blocks until all the containers specified by reqs have stopped, then print their exit codes.
func Wait(ctx context.Context, client *containerd.Client, reqs []string, options types.ContainerWaitOptions) error {
	var containers []containerd.Container
	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			containers = append(containers, found.Container)
			return nil
		},
	}

	// check if all containers from `reqs` exist
	if err := walker.WalkAll(ctx, reqs, false); err != nil {
		return err
	}

	var allErr error
	w := options.Stdout
	for _, container := range containers {
		code, waitErr := WaitContainer(ctx, container)
		fmt.Fprintln(w, code)
		if waitErr != nil {
			allErr = multierror.Append(allErr, waitErr)
		}
	}
	return allErr
}

func WaitContainer(ctx context.Context, container containerd.Container) (code int64, err error) {
	task, err := container.Task(ctx, nil)
	if err != nil {
		return -1, err
	}

	statusC, err := task.Wait(ctx)
	if err != nil {
		return -1, err
	}

	status := <-statusC
	rawcode, _, err := status.Result()
	return int64(rawcode), err
}
