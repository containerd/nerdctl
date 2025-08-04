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

package manifestutil

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

// ParseManifest parses manifest data based on media type
func ParseManifest(mediaType string, data []byte) (interface{}, error) {
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

// CreateResolver creates a resolver for registry operations
func CreateResolver(ctx context.Context, domain string, globalOptions types.GlobalCommandOptions, insecure bool) (remotes.Resolver, error) {
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

// FetchManifestData fetches manifest descriptor and data from the registry
func FetchManifestData(ctx context.Context, resolver remotes.Resolver, ref string) (ocispec.Descriptor, []byte, error) {
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

// GetManifest returns manifest, descriptor, and raw data in one call
func GetManifest(ctx context.Context, parsedRef *referenceutil.ImageReference, globalOptions types.GlobalCommandOptions, insecure bool) (interface{}, ocispec.Descriptor, []byte, error) {
	resolver, err := CreateResolver(ctx, parsedRef.Domain, globalOptions, insecure)
	if err != nil {
		return nil, ocispec.Descriptor{}, nil, fmt.Errorf("failed to create resolver: %w", err)
	}

	desc, data, err := FetchManifestData(ctx, resolver, parsedRef.String())
	if err != nil {
		return nil, ocispec.Descriptor{}, nil, err
	}

	manifest, err := ParseManifest(desc.MediaType, data)
	if err != nil {
		return nil, ocispec.Descriptor{}, nil, err
	}

	return manifest, desc, data, nil
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

// CreateManifestEntry creates a DockerManifestEntry with proper ManifestStruct
func CreateManifestEntry(parsedRef *referenceutil.ImageReference, desc ocispec.Descriptor, rawData []byte) (manifesttypes.DockerManifestEntry, error) {
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

	manifest, err := ParseManifest(desc.MediaType, rawData)
	if err != nil {
		return manifesttypes.DockerManifestEntry{}, fmt.Errorf("failed to parse manifest: %w", err)
	}

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

// getPlatformFromConfig return platform information from the config blob
func getPlatformFromConfig(ctx context.Context, resolver remotes.Resolver, ref string, configDesc ocispec.Descriptor) (*ocispec.Platform, error) {
	fetcher, err := resolver.Fetcher(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("failed to create fetcher: %w", err)
	}

	rc, err := fetcher.Fetch(ctx, configDesc)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch config: %w", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("failed to read config data: %w", err)
	}

	var config ocispec.Image
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	return &config.Platform, nil

}

// GetPlatform return the platform information from manifest config
func GetPlatform(ctx context.Context, domain string, globalOptions types.GlobalCommandOptions, insecure bool, ref string, manifest interface{}) (*ocispec.Platform, error) {
	resolver, err := CreateResolver(ctx, domain, globalOptions, insecure)
	if err != nil {
		return nil, fmt.Errorf("failed to create resolver: %w", err)
	}

	if ociManifest, ok := manifest.(manifesttypes.OCIManifestStruct); ok {
		if ociManifest.Config.Digest != "" {
			platform, err := getPlatformFromConfig(ctx, resolver, ref, ociManifest.Config)
			if err == nil && platform != nil {
				return platform, nil
			}
		}
	}

	if dockerManifest, ok := manifest.(manifesttypes.DockerManifestStruct); ok {
		if dockerManifest.Config.Digest != "" {
			platform, err := getPlatformFromConfig(ctx, resolver, ref, dockerManifest.Config)
			if err == nil && platform != nil {
				return platform, nil
			}
		}
	}

	return &ocispec.Platform{}, nil
}
