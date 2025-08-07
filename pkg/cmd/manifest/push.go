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
	"strings"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/remotes"
	"github.com/containerd/errdefs"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/manifeststore"
	"github.com/containerd/nerdctl/v2/pkg/manifesttypes"
	"github.com/containerd/nerdctl/v2/pkg/manifestutil"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
)

func Push(ctx context.Context, listRef string, options types.ManifestPushOptions) error {
	parsedTargetRef, err := referenceutil.Parse(listRef)
	if err != nil {
		return fmt.Errorf("failed to parse target reference %s: %w", listRef, err)
	}

	manifestStore, err := manifeststore.NewStore(options.GOptions.DataRoot)
	if err != nil {
		return fmt.Errorf("failed to create manifest store: %w", err)
	}

	manifests, err := manifestStore.GetList(parsedTargetRef)
	if err != nil {
		return fmt.Errorf("failed to get manifests: %w", err)
	}

	if len(manifests) == 0 {
		return fmt.Errorf("no manifests found for %s", listRef)
	}

	resolver, err := manifestutil.CreateResolver(ctx, parsedTargetRef.Domain, options.GOptions, options.Insecure)
	if err != nil {
		return fmt.Errorf("failed to create resolver: %w", err)
	}

	if err := pushIndividualManifests(ctx, resolver, manifests, parsedTargetRef, options); err != nil {
		return fmt.Errorf("failed to push individual manifests: %w", err)
	}

	manifestList, err := buildManifestList(manifests)
	if err != nil {
		return fmt.Errorf("failed to build manifest list: %w", err)
	}

	digest, err := pushManifestList(ctx, resolver, parsedTargetRef, manifestList)
	if err != nil {
		return fmt.Errorf("failed to push manifest list: %w", err)
	}

	fmt.Fprintln(options.Stdout, digest)

	if options.Purge {
		if err := manifestStore.Remove(parsedTargetRef); err != nil {
			return fmt.Errorf("failed to remove manifest list from store: %w", err)
		}
	}

	return nil
}

func buildManifestList(manifests []*manifesttypes.DockerManifestEntry) (manifesttypes.DockerManifestList, error) {
	if len(manifests) == 0 {
		return manifesttypes.DockerManifestList{}, fmt.Errorf("no manifests to build list from")
	}

	var descriptors []manifesttypes.DockerManifestDescriptor
	useOCIIndex := false

	for _, manifest := range manifests {
		if manifest.Descriptor.Platform == nil ||
			manifest.Descriptor.Platform.Architecture == "" ||
			manifest.Descriptor.Platform.OS == "" {
			return manifesttypes.DockerManifestList{}, fmt.Errorf("manifest %s must have an OS and Architecture to be pushed to a registry", manifest.Ref)
		}

		if manifest.Descriptor.MediaType == ocispec.MediaTypeImageManifest {
			useOCIIndex = true
		}

		descriptors = append(descriptors, manifesttypes.DockerManifestDescriptor{
			MediaType: manifest.Descriptor.MediaType,
			Size:      manifest.Descriptor.Size,
			Digest:    manifest.Descriptor.Digest,
			Platform:  *manifest.Descriptor.Platform,
		})
	}
	manifestList := manifesttypes.DockerManifestList{
		SchemaVersion: 2,
		MediaType:     images.MediaTypeDockerSchema2ManifestList,
		Manifests:     descriptors,
	}
	if useOCIIndex {
		manifestList.MediaType = ocispec.MediaTypeImageIndex
	}

	return manifestList, nil
}

func pushIndividualManifests(ctx context.Context, resolver remotes.Resolver, manifests []*manifesttypes.DockerManifestEntry, targetRef *referenceutil.ImageReference, options types.ManifestPushOptions) error {
	targetDomain := targetRef.Domain
	targetRepo := targetRef.Path

	for _, manifest := range manifests {
		manifestRef, err := referenceutil.Parse(manifest.Ref)
		if err != nil {
			return fmt.Errorf("failed to parse manifest reference %s: %w", manifest.Ref, err)
		}

		var targetManifestRef string
		if manifestRef.Domain != targetDomain {
			targetManifestRef = fmt.Sprintf("%s/%s@%s", targetDomain, manifestRef.Path, manifest.Descriptor.Digest)
		} else {
			targetManifestRef = fmt.Sprintf("%s/%s@%s", targetDomain, targetRepo, manifest.Descriptor.Digest)
		}

		if err := pushManifest(ctx, resolver, targetManifestRef, manifest); err != nil {
			return fmt.Errorf("failed to push manifest %s: %w", targetManifestRef, err)
		}

		fmt.Fprintf(options.Stdout, "Pushed ref %s with digest: %s\n", targetManifestRef, manifest.Descriptor.Digest)
	}

	return nil
}

func pushManifest(ctx context.Context, resolver remotes.Resolver, ref string, manifest *manifesttypes.DockerManifestEntry) error {
	rawData, err := base64.StdEncoding.DecodeString(manifest.Raw)
	if err != nil {
		return fmt.Errorf("failed to decode manifest data: %w", err)
	}

	pusher, err := resolver.Pusher(ctx, ref)
	if err != nil {
		return fmt.Errorf("failed to create pusher: %w", err)
	}

	writer, err := pusher.Push(ctx, manifest.Descriptor)
	if err != nil {
		if errdefs.IsAlreadyExists(err) || strings.Contains(err.Error(), "already exists") {
			return nil
		}
		return fmt.Errorf("failed to create content writer: %w", err)
	}
	defer writer.Close()

	if _, err := writer.Write(rawData); err != nil {
		return fmt.Errorf("failed to write manifest data: %w", err)
	}

	if err := writer.Commit(ctx, manifest.Descriptor.Size, manifest.Descriptor.Digest); err != nil {
		if errdefs.IsAlreadyExists(err) || strings.Contains(err.Error(), "already exists") {
			return nil
		}
		return fmt.Errorf("failed to commit manifest: %w", err)
	}

	return nil
}

func pushManifestList(ctx context.Context, resolver remotes.Resolver, targetRef *referenceutil.ImageReference, manifestList manifesttypes.DockerManifestList) (digest.Digest, error) {
	data, err := json.MarshalIndent(manifestList, "", "   ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal manifest list: %w", err)
	}

	dgst := digest.FromBytes(data)

	desc := ocispec.Descriptor{
		MediaType: manifestList.MediaType,
		Size:      int64(len(data)),
		Digest:    dgst,
	}

	pusher, err := resolver.Pusher(ctx, targetRef.String())
	if err != nil {
		return "", fmt.Errorf("failed to create pusher: %w", err)
	}

	writer, err := pusher.Push(ctx, desc)
	if err != nil {
		if errdefs.IsAlreadyExists(err) || strings.Contains(err.Error(), "already exists") {
			return dgst, nil
		}
		return "", fmt.Errorf("failed to create content writer: %w", err)
	}
	defer writer.Close()

	if _, err := writer.Write(data); err != nil {
		return "", fmt.Errorf("failed to write manifest list data: %w", err)
	}

	if err := writer.Commit(ctx, desc.Size, desc.Digest); err != nil {
		if errdefs.IsAlreadyExists(err) || strings.Contains(err.Error(), "already exists") {
			return dgst, nil
		}
		return "", fmt.Errorf("failed to commit manifest list: %w", err)
	}

	return dgst, nil
}
