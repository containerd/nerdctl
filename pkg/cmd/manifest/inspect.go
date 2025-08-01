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
	"io"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/containerd/containerd/v2/core/images"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/manifesttypes"
	"github.com/containerd/nerdctl/v2/pkg/manifestutil"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
)

func Inspect(ctx context.Context, rawRef string, options types.ManifestInspectOptions) ([]interface{}, error) {
	parsedRef, err := referenceutil.Parse(rawRef)
	if err != nil {
		return nil, fmt.Errorf("failed to parse reference: %w", err)
	}

	manifest, desc, rawData, err := manifestutil.GetManifest(ctx, parsedRef, options.GOptions, options.Insecure)
	if err != nil {
		return nil, err
	}

	if options.Verbose {
		return formatVerboseOutput(ctx, parsedRef, manifest, desc, rawData, options.Insecure)
	}

	// Return manifest wrapped in array for formatting compatibility
	return []interface{}{manifest}, nil
}

// formatVerboseOutput formats manifest data in Docker-compatible verbose format
func formatVerboseOutput(ctx context.Context, parsedRef *referenceutil.ImageReference, manifest interface{}, desc ocispec.Descriptor, rawData []byte, insecure bool) ([]interface{}, error) {
	switch desc.MediaType {
	case ocispec.MediaTypeImageIndex:
		index, ok := manifest.(manifesttypes.OCIIndexStruct)
		if !ok {
			return nil, fmt.Errorf("expected ocispec.Index for OCI index")
		}
		return verboseEntriesForManifests(ctx, parsedRef, index.Manifests, insecure)

	case images.MediaTypeDockerSchema2ManifestList:
		di, ok := manifest.(manifesttypes.DockerManifestListStruct)
		if !ok {
			return nil, fmt.Errorf("expected DockerManifestListStruct for Docker manifest list")
		}
		return verboseEntriesForManifests(ctx, parsedRef, di.Manifests, insecure)

	default:
		entry, err := manifestutil.CreateManifestEntry(parsedRef, desc, rawData)
		if err != nil {
			return nil, err
		}
		return []interface{}{entry}, nil
	}
}

// verboseEntriesForManifests fetches and formats verbose entries for a list of descriptors
func verboseEntriesForManifests(ctx context.Context, parsedRef *referenceutil.ImageReference, manifests []ocispec.Descriptor, insecure bool) ([]interface{}, error) {

	resolver, err := manifestutil.CreateResolver(ctx, parsedRef.Domain, types.GlobalCommandOptions{}, insecure)
	if err != nil {
		return nil, fmt.Errorf("failed to create resolver: %w", err)
	}

	fetcher, err := resolver.Fetcher(ctx, parsedRef.String())
	if err != nil {
		return nil, fmt.Errorf("failed to create fetcher: %w", err)
	}

	entries := make([]interface{}, 0, len(manifests))

	for _, mdesc := range manifests {
		rc, err := fetcher.Fetch(ctx, mdesc)
		if err != nil {
			return nil, err
		}
		defer rc.Close()

		data, err := io.ReadAll(rc)
		if err != nil {
			return nil, err
		}

		entry, err := manifestutil.CreateManifestEntry(parsedRef, mdesc, data)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	return entries, nil
}
