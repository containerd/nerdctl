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

package system

import (
	"context"
	"fmt"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/cmd/builder"
	"github.com/containerd/nerdctl/pkg/cmd/container"
	"github.com/containerd/nerdctl/pkg/cmd/image"
	"github.com/containerd/nerdctl/pkg/cmd/network"
	"github.com/containerd/nerdctl/pkg/cmd/volume"
)

// Prune will remove all unused containers, networks,
// images (dangling only or both dangling and unreferenced), and optionally, volumes.
func Prune(ctx context.Context, client *containerd.Client, options types.SystemPruneOptions) error {
	if err := container.Prune(ctx, client, types.ContainerPruneOptions{
		GOptions: options.GOptions,
		Stdout:   options.Stdout,
	}); err != nil {
		return err
	}
	if err := network.Prune(ctx, client, types.NetworkPruneOptions{
		GOptions:             options.GOptions,
		NetworkDriversToKeep: options.NetworkDriversToKeep,
		Stdout:               options.Stdout,
	}); err != nil {
		return err
	}
	if options.Volumes {
		if err := volume.Prune(ctx, client, types.VolumePruneOptions{
			GOptions: options.GOptions,
			Force:    true,
			Stdout:   options.Stdout,
		}); err != nil {
			return err
		}
	}
	if err := image.Prune(ctx, client, types.ImagePruneOptions{
		Stdout:   options.Stdout,
		GOptions: options.GOptions,
		All:      true,
	}); err != nil {
		return nil
	}
	prunedObjects, err := builder.Prune(ctx, types.BuilderPruneOptions{
		Stderr:       options.Stderr,
		GOptions:     options.GOptions,
		All:          options.All,
		BuildKitHost: options.BuildKitHost,
	})
	if err != nil {
		return err
	}

	if len(prunedObjects) > 0 {
		fmt.Fprintln(options.Stdout, "Deleted build cache objects:")
		for _, item := range prunedObjects {
			fmt.Fprintln(options.Stdout, item.ID)
		}
	}

	// TODO: print total reclaimed space

	return nil
}
