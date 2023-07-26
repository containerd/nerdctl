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
	"io"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/tarutil"
)

func Export(ctx context.Context, client *containerd.Client, args []string, w io.Writer) error {
	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			container := found.Container
			c, err := container.Info(ctx)
			if err != nil {
				return err
			}
			return performWithBaseFS(ctx, client, c, func(root string) error {
				tb := tarutil.NewTarballer(w)
				return tb.Tar(root)
			})

		},
	}
	req := args[0]
	n, err := walker.Walk(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to export container %s: %w", req, err)
	} else if n == 0 {
		return fmt.Errorf("no such container %s", req)
	}
	return nil
}

// performWithBaseFS will execute a given function with respect to the root filesystem of a container.
// copied over from: https://github.com/moby/moby/blob/master/daemon/containerd/image_exporter.go#L24
func performWithBaseFS(ctx context.Context, client *containerd.Client, c containers.Container, fn func(root string) error) error {
	mounts, err := client.SnapshotService(c.Snapshotter).Mounts(ctx, c.SnapshotKey)
	if err != nil {
		return err
	}
	return mount.WithTempMount(ctx, mounts, fn)
}
