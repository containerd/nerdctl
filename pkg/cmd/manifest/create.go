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

package manifest

import (
	"context"
	"fmt"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/containerd/containerd/v2/core/images"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/manifeststore"
	"github.com/containerd/nerdctl/v2/pkg/manifestutil"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
)

// Create creates a local manifest list/index
func Create(ctx context.Context, listRef string, manifestRefs []string, options types.ManifestCreateOptions) (string, error) {
	parsedListRef, err := referenceutil.Parse(listRef)
	if err != nil {
		return "", fmt.Errorf("failed to parse list reference: %w", err)
	}

	manifestStore, err := manifeststore.NewStore(options.GOptions.DataRoot)
	if err != nil {
		return "", fmt.Errorf("failed to create manifest store: %w", err)
	}

	existingManifests, err := manifestStore.GetList(parsedListRef)
	if err == nil && len(existingManifests) > 0 && !options.Amend {
		return "", fmt.Errorf("refusing to amend an existing manifest list with no --amend flag")
	}

	for _, manifestRef := range manifestRefs {
		parsedRef, err := referenceutil.Parse(manifestRef)
		if err != nil {
			return "", fmt.Errorf("failed to parse manifest reference %s: %w", manifestRef, err)
		}

		manifest, desc, rawData, err := manifestutil.GetManifest(ctx, parsedRef, options.GOptions, options.Insecure)
		if err != nil {
			return "", fmt.Errorf("failed to fetch manifest %s: %w", manifestRef, err)
		}

		// Check if the manifest is manifest list
		if desc.MediaType == images.MediaTypeDockerSchema2ManifestList || desc.MediaType == ocispec.MediaTypeImageIndex {
			return "", fmt.Errorf("%s is a manifest list", manifestRef)
		}

		imageManifest, err := manifestutil.CreateManifestEntry(parsedRef, desc, rawData)
		if err != nil {
			return "", fmt.Errorf("failed to create manifest entry for %s: %w", manifestRef, err)
		}

		// Get platform information from config
		if desc.MediaType == ocispec.MediaTypeImageManifest || desc.MediaType == images.MediaTypeDockerSchema2Manifest {
			platform, err := manifestutil.GetPlatform(ctx, parsedRef.Domain, options.GOptions, options.Insecure, manifestRef, manifest)
			if err != nil {
				return "", fmt.Errorf("failed to extract platform for %s: %w", manifestRef, err)
			}
			imageManifest.Descriptor.Platform = platform
		}

		if err := manifestStore.Save(parsedListRef, parsedRef, &imageManifest); err != nil {
			return "", fmt.Errorf("failed to store manifest %s: %w", manifestRef, err)
		}
	}

	return listRef, nil
}
