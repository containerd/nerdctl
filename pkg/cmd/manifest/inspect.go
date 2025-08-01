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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/remotes"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/imgutil/dockerconfigresolver"
	"github.com/containerd/nerdctl/v2/pkg/manifesttypes"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
)

// manifestParser defines a function type for parsing manifest data
type manifestParser func([]byte) (interface{}, error)

// manifestParsers maps media types to their parsing functions
var manifestParsers = map[string]manifestParser{
	ocispec.MediaTypeImageManifest:            parseOCIManifest,
	images.MediaTypeDockerSchema2Manifest:     parseDockerManifest,
	images.MediaTypeDockerSchema2ManifestList: parseDockerManifestList,
	ocispec.MediaTypeImageIndex:               parseOCIIndex,
}

// getManifestFieldName returns the appropriate field name based on media type
func getManifestFieldName(mediaType string) string {
	switch mediaType {
	case images.MediaTypeDockerSchema2Manifest:
		return "SchemaV2Manifest"
	case ocispec.MediaTypeImageManifest:
		return "OCIManifest"
	default:
		return "ManifestStruct"
	}
}

func Inspect(ctx context.Context, rawRef string, options types.ManifestInspectOptions) ([]interface{}, error) {
	manifest, desc, rawData, err := getManifest(ctx, rawRef, options)
	if err != nil {
		return nil, err
	}

	if options.Verbose {
		return formatVerboseOutput(ctx, rawRef, manifest, desc, rawData, options.Insecure)
	}

	// Return manifest wrapped in array for formatting compatibility
	return []interface{}{manifest}, nil
}

// formatVerboseOutput formats manifest data in Docker-compatible verbose format
func formatVerboseOutput(ctx context.Context, rawRef string, manifest interface{}, desc ocispec.Descriptor, rawData []byte, insecure bool) ([]interface{}, error) {
	switch desc.MediaType {
	case ocispec.MediaTypeImageIndex:
		index, ok := manifest.(manifesttypes.OCIIndexStruct)
		if !ok {
			return nil, fmt.Errorf("expected ocispec.Index for OCI index")
		}
		return verboseEntriesForManifests(ctx, rawRef, index.Manifests, insecure)

	case images.MediaTypeDockerSchema2ManifestList:
		di, ok := manifest.(manifesttypes.DockerManifestListStruct)
		if !ok {
			return nil, fmt.Errorf("expected DockerManifestListStruct for Docker manifest list")
		}
		return verboseEntriesForManifests(ctx, rawRef, di.Manifests, insecure)

	default:
		// Single manifest
		entry, err := createManifestEntry(rawRef, desc, rawData)
		if err != nil {
			return nil, err
		}
		return []interface{}{entry}, nil
	}
}

// createManifestEntry creates a DockerManifestEntry with proper ManifestStruct
func createManifestEntry(rawRef string, desc ocispec.Descriptor, rawData []byte) (manifesttypes.DockerManifestEntry, error) {
	parsedRef, err := referenceutil.Parse(rawRef)
	if err != nil {
		return manifesttypes.DockerManifestEntry{}, fmt.Errorf("failed to parse reference: %w", err)
	}

	var ref string
	if parsedRef.Digest != "" {
		ref = parsedRef.String()
	} else {
		ref = fmt.Sprintf("%s@%s", parsedRef.String(), desc.Digest.String())
	}

	entry := manifesttypes.DockerManifestEntry{
		Ref:        ref,
		Descriptor: desc,
		Raw:        base64.StdEncoding.EncodeToString(rawData),
	}

	// Parse manifest data based on media type
	parser, exists := manifestParsers[desc.MediaType]
	if !exists {
		return manifesttypes.DockerManifestEntry{}, fmt.Errorf("unsupported media type: %s", desc.MediaType)
	}

	manifest, err := parser(rawData)
	if err != nil {
		return manifesttypes.DockerManifestEntry{}, fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Set the appropriate manifest field based on media type
	fieldName := getManifestFieldName(desc.MediaType)
	switch fieldName {
	case "SchemaV2Manifest":
		entry.SchemaV2Manifest = manifest
	case "OCIManifest":
		entry.OCIManifest = manifest
	}

	// Special handling for OCI manifests to match Docker output
	if desc.MediaType == ocispec.MediaTypeImageManifest {
		entry.Descriptor.Annotations = nil
	}

	return entry, nil
}

// verboseEntriesForManifests fetches and formats verbose entries for a list of descriptors
func verboseEntriesForManifests(ctx context.Context, rawRef string, manifests []ocispec.Descriptor, insecure bool) ([]interface{}, error) {
	parsedRef, err := referenceutil.Parse(rawRef)
	if err != nil {
		return nil, fmt.Errorf("failed to parse reference: %w", err)
	}

	resolver, err := createResolver(ctx, parsedRef.Domain, types.GlobalCommandOptions{}, insecure)
	if err != nil {
		return nil, fmt.Errorf("failed to create resolver: %w", err)
	}

	fetcher, err := resolver.Fetcher(ctx, parsedRef.String())
	if err != nil {
		return nil, fmt.Errorf("failed to create fetcher: %w", err)
	}

	return fetchAndCreateEntries(ctx, fetcher, rawRef, manifests)
}

// fetchAndCreateEntries fetches multiple manifests and creates DockerManifestEntry objects
func fetchAndCreateEntries(ctx context.Context, fetcher remotes.Fetcher, rawRef string, manifests []ocispec.Descriptor) ([]interface{}, error) {
	entries := make([]interface{}, 0, len(manifests))

	for _, mdesc := range manifests {
		entry, err := fetchAndCreateEntry(ctx, fetcher, rawRef, mdesc)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// fetchAndCreateEntry fetches a single manifest and creates a DockerManifestEntry
func fetchAndCreateEntry(ctx context.Context, fetcher remotes.Fetcher, rawRef string, desc ocispec.Descriptor) (manifesttypes.DockerManifestEntry, error) {
	rc, err := fetcher.Fetch(ctx, desc)
	if err != nil {
		return manifesttypes.DockerManifestEntry{}, err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return manifesttypes.DockerManifestEntry{}, err
	}

	entry, err := createManifestEntry(rawRef, desc, data)
	if err != nil {
		return manifesttypes.DockerManifestEntry{}, err
	}

	return entry, nil
}

// createResolver creates a resolver for registry operations
func createResolver(ctx context.Context, domain string, globalOptions types.GlobalCommandOptions, insecure bool) (remotes.Resolver, error) {
	dOpts := buildResolverOptions(globalOptions, insecure)

	resolver, err := dockerconfigresolver.New(ctx, domain, dOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create resolver: %w", err)
	}

	return resolver, nil
}

// buildResolverOptions builds resolver options based on global options and security settings
func buildResolverOptions(globalOptions types.GlobalCommandOptions, insecure bool) []dockerconfigresolver.Opt {
	var dOpts []dockerconfigresolver.Opt

	if insecure {
		dOpts = append(dOpts, dockerconfigresolver.WithSkipVerifyCerts(true))
	}
	dOpts = append(dOpts, dockerconfigresolver.WithHostsDirs(globalOptions.HostsDir))

	return dOpts
}

// getManifest returns manifest, descriptor, and raw data in one call
func getManifest(ctx context.Context, rawRef string, options types.ManifestInspectOptions) (interface{}, ocispec.Descriptor, []byte, error) {
	parsedRef, err := referenceutil.Parse(rawRef)
	if err != nil {
		return nil, ocispec.Descriptor{}, nil, fmt.Errorf("failed to parse reference: %w", err)
	}

	resolver, err := createResolver(ctx, parsedRef.Domain, options.GOptions, options.Insecure)
	if err != nil {
		return nil, ocispec.Descriptor{}, nil, fmt.Errorf("failed to create resolver: %w", err)
	}

	desc, data, err := fetchManifestData(ctx, resolver, parsedRef.String())
	if err != nil {
		return nil, ocispec.Descriptor{}, nil, err
	}

	manifest, err := parseManifest(desc.MediaType, data)
	if err != nil {
		return nil, ocispec.Descriptor{}, nil, err
	}

	return manifest, desc, data, nil
}

// fetchManifestData fetches manifest descriptor and data from the registry
func fetchManifestData(ctx context.Context, resolver remotes.Resolver, ref string) (ocispec.Descriptor, []byte, error) {
	_, desc, err := resolver.Resolve(ctx, ref)
	if err != nil {
		return ocispec.Descriptor{}, nil, fmt.Errorf("failed to resolve %s: %w", ref, err)
	}

	fetcher, err := resolver.Fetcher(ctx, ref)
	if err != nil {
		return ocispec.Descriptor{}, nil, fmt.Errorf("failed to create fetcher: %w", err)
	}

	rc, err := fetcher.Fetch(ctx, desc)
	if err != nil {
		return ocispec.Descriptor{}, nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return ocispec.Descriptor{}, nil, fmt.Errorf("failed to read manifest data: %w", err)
	}

	return desc, data, nil
}

// parseManifest parses manifest data based on media type
func parseManifest(mediaType string, data []byte) (interface{}, error) {
	if parser, exists := manifestParsers[mediaType]; exists {
		return parser(data)
	}
	return nil, fmt.Errorf("unsupported media type: %s", mediaType)
}

// parseOCIManifest parses OCI manifest data
func parseOCIManifest(data []byte) (interface{}, error) {
	var ociManifest manifesttypes.OCIManifestStruct
	if err := json.Unmarshal(data, &ociManifest); err != nil {
		return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
	}
	return ociManifest, nil
}

// parseDockerManifest parses Docker manifest data
func parseDockerManifest(data []byte) (interface{}, error) {
	var dockerManifest manifesttypes.DockerManifestStruct
	if err := json.Unmarshal(data, &dockerManifest); err != nil {
		return nil, fmt.Errorf("failed to unmarshal docker manifest: %w", err)
	}
	return dockerManifest, nil
}

// parseDockerManifestList parses Docker manifest list data
func parseDockerManifestList(data []byte) (interface{}, error) {
	var manifestList manifesttypes.DockerManifestListStruct
	if err := json.Unmarshal(data, &manifestList); err != nil {
		return nil, fmt.Errorf("failed to unmarshal docker index: %w", err)
	}
	return manifestList, nil
}

// parseOCIIndex parses OCI index data
func parseOCIIndex(data []byte) (interface{}, error) {
	var index manifesttypes.OCIIndexStruct
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("failed to unmarshal index: %w", err)
	}
	return index, nil
}
