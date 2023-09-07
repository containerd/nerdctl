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
	"fmt"
	"io"
	"sync"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/containerutil"
	"github.com/containerd/nerdctl/pkg/labels"
	"golang.org/x/sync/errgroup"
)

// Pause pauses service containers belonging to `services`.
func (c *Composer) Pause(ctx context.Context, services []string, writer io.Writer) error {
	serviceNames, err := c.ServiceNames(services...)
	if err != nil {
		return err
	}
	containers, err := c.Containers(ctx, serviceNames...)
	if err != nil {
		return err
	}

	var mu sync.Mutex

	eg, ctx := errgroup.WithContext(ctx)
	for _, container := range containers {
		container := container
		eg.Go(func() error {
			if err := containerutil.Pause(ctx, c.client, container.ID()); err != nil {
				return err
			}
			info, err := container.Info(ctx, containerd.WithoutRefreshedMetadata)
			if err != nil {
				return err
			}

			mu.Lock()
			defer mu.Unlock()
			_, err = fmt.Fprintln(writer, info.Labels[labels.Name])

			return err
		})
	}

	return eg.Wait()
}

// Unpause unpauses service containers belonging to `services`.
func (c *Composer) Unpause(ctx context.Context, services []string, writer io.Writer) error {
	serviceNames, err := c.ServiceNames(services...)
	if err != nil {
		return err
	}
	containers, err := c.Containers(ctx, serviceNames...)
	if err != nil {
		return err
	}

	var mu sync.Mutex

	eg, ctx := errgroup.WithContext(ctx)
	for _, container := range containers {
		container := container
		eg.Go(func() error {
			if err := containerutil.Unpause(ctx, c.client, container.ID()); err != nil {
				return err
			}
			info, err := container.Info(ctx, containerd.WithoutRefreshedMetadata)
			if err != nil {
				return err
			}

			mu.Lock()
			defer mu.Unlock()
			_, err = fmt.Fprintln(writer, info.Labels[labels.Name])

			return err
		})
	}

	return eg.Wait()
}
