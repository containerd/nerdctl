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
	"regexp"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/formatter"
	"github.com/containerd/nerdctl/v2/pkg/imageinspector"
	"github.com/containerd/nerdctl/v2/pkg/imgutil"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
)

// Inspect prints detailed information of each image in `images`.
func Inspect(ctx context.Context, client *containerd.Client, identifiers []string, options types.ImageInspectOptions) error {
	objects := make(map[string]*dockercompat.Image)
	var entries []interface{}

	// Construct the filters
	var filters []string
	for _, identifier := range identifiers {
		if canonicalRef, err := referenceutil.ParseAny(identifier); err == nil {
			filters = append(filters, fmt.Sprintf("name==%s", canonicalRef.String()))
		}
		filters = append(filters,
			fmt.Sprintf("name==%s", identifier),
			fmt.Sprintf("target.digest~=^sha256:%s.*$", regexp.QuoteMeta(identifier)),
			fmt.Sprintf("target.digest~=^%s.*$", regexp.QuoteMeta(identifier)),
		)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Query containerd image service to retrieve a slice of containerd images
	images, err := client.ImageService().List(ctx, filters...)
	if err != nil {
		return fmt.Errorf("image inspect errored while trying to query containerd ImageService: %w", err)
	}

	// Iterate over the results
	for _, image := range images {
		// Query each image with a timeout
		nativeImage, err := imageinspector.Inspect(ctx, client, image, options.GOptions.Snapshotter)
		if err != nil {
			return fmt.Errorf("image inspect errored while trying to inspect image %s: %w", image.Name, err)
		}
		// Get the image digest
		newDigest := nativeImage.ImageConfigDesc.Digest.String()
		// If we do not know about this image yet, add it to our collection
		if objects[newDigest] == nil {
			d, err := dockercompat.ImageFromNative(nativeImage)
			if err != nil {
				return fmt.Errorf("image inspect failed to marshall native image: %w", err)
			}
			objects[newDigest] = d
			entries = append(entries, d)
		} else {
			// If we do know about this digest already, add the tags to the existing entry
			repository, tag := imgutil.ParseRepoTag(nativeImage.Image.Name)
			objects[newDigest].RepoTags = append(objects[newDigest].RepoTags, fmt.Sprintf("%s:%s", repository, tag))
		}
	}

	if len(entries) > 0 {
		if formatErr := formatter.FormatSlice(options.Format, options.Stdout, entries); formatErr != nil {
			log.G(ctx).Error(formatErr)
		}
	}
	return nil
}
