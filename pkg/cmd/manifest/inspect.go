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
	"regexp"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/containerdutil"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/v2/pkg/manifestinspector"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
)

// DockerManifestEntry represents a single manifest entry in Docker's verbose format
type DockerManifestEntry struct {
	Ref              string             `json:"Ref"`
	Descriptor       ocispec.Descriptor `json:"Descriptor"`
	Raw              string             `json:"Raw"`
	SchemaV2Manifest ocispec.Manifest   `json:"SchemaV2Manifest"`
}

func Inspect(ctx context.Context, client *containerd.Client, rawRef string, options types.ManifestInspectOptions) ([]any, error) {
	manifest, err := getManifest(ctx, client, rawRef, options)
	if err != nil {
		return nil, err
	}

	// If manifest is already an array (verbose tag case), return as is
	if manifestArray, isArray := manifest.([]DockerManifestEntry); isArray {
		result := make([]any, len(manifestArray))
		for i, entry := range manifestArray {
			result[i] = entry
		}
		return result, nil
	}

	// If manifest is not an array, wrap it in array for formatting compatibility
	return []any{manifest}, nil
}

func inspectIdentifier(ctx context.Context, client *containerd.Client, identifier string) ([]images.Image, string, string, error) {
	parsedReference, err := referenceutil.Parse(identifier)
	if err != nil {
		return nil, "", "", err
	}
	digest := ""
	if parsedReference.Digest != "" {
		digest = parsedReference.Digest.String()
	}
	name := parsedReference.Name()
	tag := parsedReference.Tag

	var filters []string
	var imageList []images.Image

	if digest == "" {
		filters = []string{fmt.Sprintf("name==%s:%s", name, tag)}
		imageList, err = client.ImageService().List(ctx, filters...)
		if err != nil {
			return nil, "", "", fmt.Errorf("containerd image service failed: %w", err)
		}
		if len(imageList) == 0 {
			digest = fmt.Sprintf("sha256:%s.*", regexp.QuoteMeta(strings.TrimPrefix(identifier, "sha256:")))
			name = ""
			tag = ""
		} else {
			digest = imageList[0].Target.Digest.String()
		}
	}
	filters = []string{fmt.Sprintf("target.digest~=^%s$", digest)}
	imageList, err = client.ImageService().List(ctx, filters...)
	if err != nil {
		return nil, "", "", fmt.Errorf("containerd image service failed: %w", err)
	}

	if len(imageList) == 0 && digest != "" {
		imageList, err = findImageByManifestDigest(ctx, client, digest)
		if err != nil {
			return nil, "", "", fmt.Errorf("find image by manifest digest failed: %w", err)
		}
	}

	return imageList, name, tag, nil
}

func getManifest(ctx context.Context, client *containerd.Client, rawRef string, options types.ManifestInspectOptions) (interface{}, error) {
	candidateImageList, _, _, err := inspectIdentifier(ctx, client, rawRef)
	if err != nil {
		return nil, err
	}

	if len(candidateImageList) == 0 {
		return nil, fmt.Errorf("no manifest found for %s", rawRef)
	}

	// Process the first (and usually only) candidate image
	entry, err := manifestinspector.Inspect(ctx, client, candidateImageList[0])
	if err != nil {
		return nil, err
	}

	// Process entry based on rawRef and verbose options to match Docker format
	processedEntry, err := processManifestEntryDockerCompat(ctx, client, entry, rawRef, options.Verbose)
	if err != nil {
		return nil, err
	}

	return processedEntry, nil
}

// processManifestEntryDockerCompat processes manifest entry to match Docker's output format
func processManifestEntryDockerCompat(ctx context.Context, client *containerd.Client, entry *native.Manifest, rawRef string, verbose bool) (interface{}, error) {
	parsedReference, err := referenceutil.Parse(rawRef)
	if err != nil {
		return nil, err
	}

	digest := ""
	if parsedReference.Digest != "" {
		digest = parsedReference.Digest.String()
	}

	provider := containerdutil.NewProvider(client)

	// If rawRef has no digest
	if digest == "" {
		if verbose {
			// verbose is true, output Docker-compatible array format
			var dockerEntries []DockerManifestEntry

			// If we have an index, process all manifests in it
			if entry.Index != nil {
				for _, manifestEntry := range entry.Manifests {
					if manifestEntry.ManifestDesc != nil && manifestEntry.Manifest != nil {
						dockerEntry, err := createDockerManifestEntry(ctx, provider, rawRef, *manifestEntry.ManifestDesc, *manifestEntry.Manifest)
						if err == nil {
							dockerEntries = append(dockerEntries, dockerEntry)
						}
					}
				}
			} else if len(entry.Manifests) > 0 {
				// No index, but we have manifests
				for _, manifestEntry := range entry.Manifests {
					if manifestEntry.ManifestDesc != nil && manifestEntry.Manifest != nil {
						dockerEntry, err := createDockerManifestEntry(ctx, provider, rawRef, *manifestEntry.ManifestDesc, *manifestEntry.Manifest)
						if err == nil {
							dockerEntries = append(dockerEntries, dockerEntry)
						}
					}
				}
			}
			return dockerEntries, nil
		}
		// verbose is false, decide output based on whether index is empty
		if entry.Index != nil {
			// index is not empty, only output Index content (like Docker)
			return entry.Index, nil
		}
		// index is empty, only output Manifest content
		if len(entry.Manifests) == 1 && entry.Manifests[0].Manifest != nil {
			// If there's only one manifest, output it directly
			return entry.Manifests[0].Manifest, nil
		}
		if len(entry.Manifests) > 0 {
			// Multiple manifests but no index - this shouldn't happen normally
			// but if it does, return the first manifest
			for _, manifestEntry := range entry.Manifests {
				if manifestEntry.Manifest != nil {
					return manifestEntry.Manifest, nil
				}
			}
		}
		return nil, fmt.Errorf("no valid manifest found")
	}

	// If rawRef has digest, find matching content
	// Check if digest matches index digest
	if entry.IndexDesc != nil && entry.IndexDesc.Digest.String() == digest {
		if verbose {
			// For verbose index digest request, return empty array (matches Docker behavior)
			return []DockerManifestEntry{}, nil
		}
		// verbose is false, only output Index if index is not empty
		if entry.Index != nil {
			return entry.Index, nil
		}
	}

	// Check if digest matches manifest digest
	for _, manifestEntry := range entry.Manifests {
		if manifestEntry.ManifestDesc != nil && manifestEntry.ManifestDesc.Digest.String() == digest {
			if verbose {
				// For specific manifest digest, Docker returns the object directly, not in an array
				dockerEntry, err := createDockerManifestEntry(ctx, provider, rawRef, *manifestEntry.ManifestDesc, *manifestEntry.Manifest)
				if err != nil {
					return nil, err
				}
				return dockerEntry, nil
			}
			// Return just the manifest content
			if manifestEntry.Manifest != nil {
				return manifestEntry.Manifest, nil
			}
			break
		}
	}
	return nil, fmt.Errorf("no matching manifest found for digest %s", digest)
}

// createDockerManifestEntry creates a DockerManifestEntry compatible with Docker's verbose format
func createDockerManifestEntry(ctx context.Context, provider content.Provider, rawRef string, desc ocispec.Descriptor, manifest ocispec.Manifest) (DockerManifestEntry, error) {
	// Read raw manifest bytes
	rawBytes, err := containerdutil.ReadBlob(ctx, provider, desc)
	if err != nil {
		return DockerManifestEntry{}, err
	}

	// Create the ref string
	ref := fmt.Sprintf("%s@%s", rawRef, desc.Digest.String())
	if strings.Contains(rawRef, "@") {
		// rawRef already contains digest
		ref = rawRef
	}

	return DockerManifestEntry{
		Ref:              ref,
		Descriptor:       desc,
		Raw:              base64.StdEncoding.EncodeToString(rawBytes),
		SchemaV2Manifest: manifest,
	}, nil
}

func findImageByManifestDigest(
	ctx context.Context,
	client *containerd.Client,
	targetDigest string,
) ([]images.Image, error) {
	var resultList []images.Image
	imageList, err := client.ImageService().List(ctx)
	if err != nil {
		return nil, err
	}
	provider := containerdutil.NewProvider(client)
	for _, img := range imageList {
		desc := img.Target
		if images.IsIndexType(desc.MediaType) {
			indexData, err := containerdutil.ReadBlob(ctx, provider, desc)
			if err != nil {
				continue
			}
			var index ocispec.Index
			if err := json.Unmarshal(indexData, &index); err != nil {
				continue
			}
			for _, mani := range index.Manifests {
				if mani.Digest.String() == targetDigest {
					resultList = append(resultList, img)
				}
			}
		}
		if images.IsManifestType(desc.MediaType) && desc.Digest.String() == targetDigest {
			resultList = append(resultList, img)
		}
	}
	return resultList, nil
}
