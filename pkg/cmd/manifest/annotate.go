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
	"errors"
	"fmt"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/manifeststore"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
	"github.com/containerd/nerdctl/v2/pkg/store"
)

func Annotate(ctx context.Context, listRef string, manifestRef string, options types.ManifestAnnotateOptions) error {
	parsedListRef, err := referenceutil.Parse(listRef)
	if err != nil {
		return fmt.Errorf("failed to parse list reference: %w", err)
	}

	parsedManifestRef, err := referenceutil.Parse(manifestRef)
	if err != nil {
		return fmt.Errorf("failed to parse manifest reference: %w", err)
	}

	manifestStore, err := manifeststore.NewStore(options.GOptions.DataRoot)
	if err != nil {
		return fmt.Errorf("failed to create manifest store: %w", err)
	}

	imageManifest, err := manifestStore.Get(parsedListRef, parsedManifestRef)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("manifest for image %s does not exist in %s", manifestRef, listRef)
		}
		return fmt.Errorf("failed to get manifest: %w", err)
	}

	if imageManifest.Descriptor.Platform == nil {
		imageManifest.Descriptor.Platform = new(ocispec.Platform)
	}

	if options.Os != "" {
		imageManifest.Descriptor.Platform.OS = options.Os
	}

	if options.Arch != "" {
		imageManifest.Descriptor.Platform.Architecture = options.Arch
	}

	if options.Variant != "" {
		imageManifest.Descriptor.Platform.Variant = options.Variant
	}

	if options.OsVersion != "" {
		imageManifest.Descriptor.Platform.OSVersion = options.OsVersion
	}

	for _, osFeature := range options.OsFeatures {
		imageManifest.Descriptor.Platform.OSFeatures = appendIfUnique(imageManifest.Descriptor.Platform.OSFeatures, osFeature)
	}

	return manifestStore.Save(parsedListRef, parsedManifestRef, imageManifest)
}

func appendIfUnique(list []string, str string) []string {
	for _, s := range list {
		if s == str {
			return list
		}
	}
	return append(list, str)
}
