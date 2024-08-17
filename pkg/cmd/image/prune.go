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

	"github.com/opencontainers/go-digest"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/log"
	"github.com/containerd/platforms"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/imgutil"
)

// Prune will remove all dangling images. If all is specified, will also remove all images not referenced by any container.
func Prune(ctx context.Context, client *containerd.Client, options types.ImagePruneOptions) error {
	var (
		imageStore   = client.ImageService()
		contentStore = client.ContentStore()
	)

	var (
		imagesToBeRemoved []images.Image
		err               error
	)

	filters := []imgutil.Filter{}
	if len(options.Filters) > 0 {
		parsedFilters, err := imgutil.ParseFilters(options.Filters)
		if err != nil {
			return err
		}
		if len(parsedFilters.Labels) > 0 {
			filters = append(filters, imgutil.FilterByLabel(ctx, client, parsedFilters.Labels))
		}
		if len(parsedFilters.Until) > 0 {
			filters = append(filters, imgutil.FilterUntil(parsedFilters.Until))
		}
	}

	if options.All {
		// Remove all unused images; not just dangling ones
		imagesToBeRemoved, err = imgutil.GetUnusedImages(ctx, client, filters...)
	} else {
		// Remove dangling images only
		imagesToBeRemoved, err = imgutil.GetDanglingImages(ctx, client, filters...)
	}
	if err != nil {
		return err
	}

	delOpts := []images.DeleteOpt{images.SynchronousDelete()}
	removedImages := make(map[string][]digest.Digest)
	for _, image := range imagesToBeRemoved {
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
