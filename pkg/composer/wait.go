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

package composer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"golang.org/x/sync/errgroup"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"
)

// WaitOptions specifies the options for Wait.
type WaitOptions struct {
	// DownProject removes the project once the containers stop.
	DownProject bool
}

// Wait blocks until the containers of `services` stop, prints the exit code of
// each of them, and returns the exit code of the last one to stop, like
// `docker compose wait`. Containers that have already stopped return right
// away. Containers that were never started have no task to wait for and are
// skipped.
func (c *Composer) Wait(ctx context.Context, services []string, options WaitOptions, stdout io.Writer) (uint32, error) {
	serviceNames, err := c.ServiceNames(services...)
	if err != nil {
		return 0, err
	}
	containers, err := c.Containers(ctx, serviceNames...)
	if err != nil {
		return 0, err
	}

	type waitTarget struct {
		id      string
		statusC <-chan containerd.ExitStatus
	}
	var targets []waitTarget
	for _, container := range containers {
		task, err := container.Task(ctx, nil)
		if err != nil {
			if errdefs.IsNotFound(err) {
				log.G(ctx).Debugf("container %s has no task, skipping it", container.ID())
				continue
			}
			return 0, err
		}
		statusC, err := task.Wait(ctx)
		if err != nil {
			return 0, err
		}
		targets = append(targets, waitTarget{id: container.ID(), statusC: statusC})
	}
	if len(targets) == 0 {
		return 0, fmt.Errorf("no started containers found for services %v", serviceNames)
	}

	var (
		mu         sync.Mutex
		statusCode uint32
	)
	eg, egCtx := errgroup.WithContext(ctx)
	for _, target := range targets {
		eg.Go(func() error {
			select {
			case status := <-target.statusC:
				code, _, err := status.Result()
				if err != nil {
					return err
				}
				mu.Lock()
				defer mu.Unlock()
				statusCode = code
				_, err = fmt.Fprintf(stdout, "container %q exited with status code %d\n", target.id, code)
				return err
			case <-egCtx.Done():
				return egCtx.Err()
			}
		})
	}
	if err := eg.Wait(); err != nil {
		return 0, err
	}

	if options.DownProject {
		if err := c.Down(ctx, DownOptions{RemoveOrphans: true}, nil); err != nil {
			return statusCode, errors.Join(fmt.Errorf("failed to remove project %s", c.project.Name), err)
		}
	}
	return statusCode, nil
}
