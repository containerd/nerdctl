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

package image

import (
	"context"
	"fmt"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/opencontainers/go-digest"
)

// Prune will remove all dangling images. If all is specified, will also remove all images not referenced by any container.
func Prune(ctx context.Context, client *containerd.Client, options types.ImagePruneOptions) error {
	var (
		imageStore     = client.ImageService()
		contentStore   = client.ContentStore()
		containerStore = client.ContainerService()
	)

	imageList, err := imageStore.List(ctx)
	if err != nil {
		return err
	}

	var filteredImages []images.Image

	if options.All {
		containerList, err := containerStore.List(ctx)
		if err != nil {
			return err
		}
		usedImages := make(map[string]struct{})
		for _, container := range containerList {
			usedImages[container.Image] = struct{}{}
		}

		for _, image := range imageList {
			if _, ok := usedImages[image.Name]; ok {
				continue
			}

			filteredImages = append(filteredImages, image)
		}
	} else {
		filteredImages = imgutil.FilterDangling(imageList, true)
	}

	delOpts := []images.DeleteOpt{images.SynchronousDelete()}
	removedImages := make(map[string][]digest.Digest)
	for _, image := range filteredImages {
		digests, err := image.RootFS(ctx, contentStore, platforms.DefaultStrict())
		if err != nil {
			log.G(ctx).WithError(err).Warnf("failed to enumerate rootfs")
		}
		if err := imageStore.Delete(ctx, image.Name, delOpts...); err != nil {
			log.G(ctx).WithError(err).Warnf("failed to delete image %s", image.Name)
			continue
		}
		removedImages[image.Name] = digests
	}

	if len(removedImages) > 0 {
		fmt.Fprintln(options.Stdout, "Deleted Images:")
		for image, digests := range removedImages {
			fmt.Fprintf(options.Stdout, "Untagged: %s\n", image)
			for _, digest := range digests {
				fmt.Fprintf(options.Stdout, "deleted: %s\n", digest)
			}
		}
		fmt.Fprintln(options.Stdout, "")
	}
	return nil
}
