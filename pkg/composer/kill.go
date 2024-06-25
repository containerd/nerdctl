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

	"golang.org/x/sync/errgroup"

	"github.com/containerd/log"
)

type KillOptions struct {
	Signal string
}

func (c *Composer) Kill(ctx context.Context, opts KillOptions, services []string) error {
	serviceNames, err := c.ServiceNames(services...)
	if err != nil {
		return err
	}
	containers, err := c.Containers(ctx, serviceNames...)
	if err != nil {
		return err
	}
	eg, ctx := errgroup.WithContext(ctx)
	for _, container := range containers {
		container := container
		eg.Go(func() error {
			args := []string{"kill", "-s", opts.Signal, container.ID()}
			if err := c.runNerdctlCmd(ctx, args...); err != nil {
				log.G(ctx).Warn(err)
				return err
			}
			return nil
		})
	}
	return eg.Wait()
}
