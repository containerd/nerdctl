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
	"encoding/json"
	"maps"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/identity"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"
	"github.com/containerd/platforms"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/containerdutil"
	"github.com/containerd/nerdctl/v2/pkg/imgutil"
)

// DiskUsage reports how much disk space the images of the current namespace use.
//
// The definitions follow Docker v29 with the containerd image store
// (moby/moby daemon/disk_usage.go and daemon/containerd/service.go):
//
//   - the size of an image is the content present in the content store plus its unpacked snapshots,
//   - TotalSize counts every snapshot and every blob once, even when several images share it,
//   - an image is active when at least one container references it,
//   - the reclaimable space is the size that is unique to the images no container references.
func DiskUsage(ctx context.Context, client *containerd.Client, gOptions types.GlobalCommandOptions, verbose bool) (types.ImageDiskUsage, error) {
	du := types.ImageDiskUsage{}

	imageList, err := List(ctx, client, nil, nil)
	if err != nil {
		return du, err
	}

	var (
		contentStore = client.ContentStore()
		provider     = containerdutil.NewProvider(client)
		snapshotter  = containerdutil.SnapshotService(client, gOptions.Snapshotter)
		containers   = imagesInUse(ctx, client)
	)

	// The image store may hold several names for the same target (repo:tag, repo@digest, and under
	// the k8s.io namespace the config digest as well). Docker reports one entry per target, so
	// collapse them here, otherwise every count and every size would be multiplied.
	uniqueImages := uniqueByTarget(imageList)

	collected := make([]*imageContents, 0, len(uniqueImages))
	index := newDiskUsageIndex()

	for _, img := range uniqueImages {
		contents, err := readImageContents(ctx, contentStore, provider, img)
		if err != nil {
			log.G(ctx).WithError(err).Warnf("failed to compute the disk usage of image %q", img.Name)
			continue
		}
		collected = append(collected, contents)
		index.add(contents, func(chainID digest.Digest) int64 {
			return snapshotUsage(ctx, snapshotter, chainID)
		})
	}

	du.TotalCount = int64(len(collected))
	du.TotalSize = index.total()

	for _, contents := range collected {
		size, sharedSize := index.sizes(contents)

		inUse := containers[contents.image.Target.Digest]
		if inUse > 0 {
			du.ActiveCount++
		} else {
			du.Reclaimable += size - sharedSize
		}

		if verbose {
			repository, tag := imgutil.ParseRepoTag(contents.image.Name)
			du.Items = append(du.Items, types.ImageDiskUsageItem{
				ID:         contents.image.Target.Digest.String(),
				Repository: repository,
				Tag:        tag,
				CreatedAt:  contents.createdAt(),
				Size:       size,
				SharedSize: sharedSize,
				Containers: inUse,
			})
		}
	}

	return du, nil
}

// imageContents is what a single image occupies on disk: the chain IDs of its unpacked snapshots
// (across every platform) and the blobs of its content that are locally present.
type imageContents struct {
	image    images.Image
	chainIDs []digest.Digest
	// blobs is the content of the manifests of the image. Docker sizes an image by walking its
	// manifests, so the index listing them is not part of what a single image is charged for.
	blobs map[digest.Digest]int64
	// indexBlobs is the content of those indexes. It is on disk, so the total counts it, but no
	// image is charged for it.
	indexBlobs map[digest.Digest]int64
	// created is when the image was built, as stated by its config. It is nil when no config says.
	created *time.Time
}

// createdAt is when the image was built. Docker reports the "created" of the image config; the
// creation time of the local image record only says when it was pulled or tagged, which would show
// an old image as brand new. It is the fallback for the images that do not state one.
func (contents *imageContents) createdAt() time.Time {
	if contents.created != nil {
		return *contents.created
	}
	return contents.image.CreatedAt
}

// diskUsageIndex records, for every snapshot and every blob, how many images hold it and how large
// it is. That is what makes the deduplicated total and the per-image shared size computable without
// a second pass over the content store.
type diskUsageIndex struct {
	layerCount map[digest.Digest]int
	blobCount  map[digest.Digest]int
	layerSize  map[digest.Digest]int64
	blobSize   map[digest.Digest]int64
	// indexSize is the content no single image is charged for. It is deduplicated by digest like
	// the rest, it just never contributes to a per-image size, so it needs no count.
	indexSize map[digest.Digest]int64
}

func newDiskUsageIndex() *diskUsageIndex {
	return &diskUsageIndex{
		layerCount: map[digest.Digest]int{},
		blobCount:  map[digest.Digest]int{},
		layerSize:  map[digest.Digest]int64{},
		blobSize:   map[digest.Digest]int64{},
		indexSize:  map[digest.Digest]int64{},
	}
}

// add accounts for one image. usage is only called the first time a snapshot is seen, so a snapshot
// shared by many images is measured once.
func (index *diskUsageIndex) add(contents *imageContents, usage func(digest.Digest) int64) {
	for _, chainID := range contents.chainIDs {
		index.layerCount[chainID]++
		if _, ok := index.layerSize[chainID]; !ok {
			index.layerSize[chainID] = usage(chainID)
		}
	}
	for dgst, size := range contents.blobs {
		index.blobCount[dgst]++
		index.blobSize[dgst] = size
	}
	maps.Copy(index.indexSize, contents.indexBlobs)
}

// total is the disk space the images take together, counting everything they share only once.
func (index *diskUsageIndex) total() int64 {
	var total int64
	for chainID := range index.layerCount {
		total += index.layerSize[chainID]
	}
	for dgst := range index.blobCount {
		total += index.blobSize[dgst]
	}
	for _, size := range index.indexSize {
		total += size
	}
	return total
}

// sizes returns what one image occupies, and how much of that is also held by another image.
func (index *diskUsageIndex) sizes(contents *imageContents) (size, sharedSize int64) {
	for _, chainID := range contents.chainIDs {
		size += index.layerSize[chainID]
		if index.layerCount[chainID] > 1 {
			sharedSize += index.layerSize[chainID]
		}
	}
	for dgst, blob := range contents.blobs {
		size += blob
		if index.blobCount[dgst] > 1 {
			sharedSize += blob
		}
	}
	return size, sharedSize
}

// readImageContents walks everything reachable from the image target that is present in the content
// store, collecting the blobs on the way and deriving the chain IDs from the image configs.
func readImageContents(ctx context.Context, store content.Store, provider content.Provider, img images.Image) (*imageContents, error) {
	contents := &imageContents{
		image:      img,
		blobs:      map[digest.Digest]int64{},
		indexBlobs: map[digest.Digest]int64{},
	}

	var manifestDescs []ocispec.Descriptor
	if err := containerdutil.WalkPresentChildren(ctx, store, img.Target, func(_ context.Context, desc ocispec.Descriptor) error {
		if images.IsIndexType(desc.MediaType) {
			contents.indexBlobs[desc.Digest] = desc.Size
			return nil
		}
		contents.blobs[desc.Digest] = desc.Size
		if images.IsManifestType(desc.MediaType) {
			manifestDescs = append(manifestDescs, desc)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	seen := map[digest.Digest]struct{}{}
	var built []buildTime
	for _, desc := range manifestDescs {
		config, err := readConfig(ctx, provider, desc)
		if err != nil {
			// Attestation manifests and manifests whose config we cannot read carry no rootfs.
			// Their content is still accounted for above, they just contribute no snapshot.
			log.G(ctx).WithError(err).Debugf("no rootfs for manifest %q of image %q", desc.Digest, img.Name)
			continue
		}
		if !isAttestationManifestDescriptor(desc) {
			built = append(built, buildTime{
				platform: manifestPlatform(desc, config),
				created:  config.Created,
			})
		}
		for _, chainID := range identity.ChainIDs(config.RootFS.DiffIDs) {
			if _, ok := seen[chainID]; ok {
				continue
			}
			seen[chainID] = struct{}{}
			contents.chainIDs = append(contents.chainIDs, chainID)
		}
	}
	contents.created = hostBuildTime(built)

	return contents, nil
}

// buildTime is when one platform of an image was built. created is nil when the config of that
// platform states no build time, which it is free not to.
type buildTime struct {
	platform ocispec.Platform
	created  *time.Time
}

// hostBuildTime picks the build time to report for a multi-platform image. The platforms of an
// index are not necessarily built together, so report the one this host would run, as Docker does
// by reading the config of the manifest its platform matcher selects. The platform decides which
// config answers, so a host manifest saying nothing is an answer too: it leaves the caller with the
// creation time of the local record rather than with the build time of another architecture.
func hostBuildTime(built []buildTime) *time.Time {
	matcher := platforms.Default()
	best := -1
	for i, candidate := range built {
		if !matcher.Match(candidate.platform) {
			continue
		}
		if best == -1 || matcher.Less(candidate.platform, built[best].platform) {
			best = i
		}
	}
	if best >= 0 {
		return built[best].created
	}

	// No platform of the image runs here (an image pulled for another architecture, say). Any
	// build time describes the image better than none.
	for _, candidate := range built {
		if candidate.created != nil {
			return candidate.created
		}
	}
	return nil
}

// manifestPlatform reports the platform of a manifest. The descriptor is authoritative: it is what
// an index selects a platform by, and it can be more specific than the config, which may declare a
// bare "linux/arm" for what the index calls linux/arm/v6 and linux/arm/v7.
func manifestPlatform(desc ocispec.Descriptor, config *ocispec.Image) ocispec.Platform {
	if desc.Platform != nil {
		return platforms.Normalize(*desc.Platform)
	}
	return platforms.Normalize(ocispec.Platform{
		OS:           config.OS,
		Architecture: config.Architecture,
		Variant:      config.Variant,
	})
}

// readConfig returns the image config referenced by the given manifest.
func readConfig(ctx context.Context, provider content.Provider, desc ocispec.Descriptor) (*ocispec.Image, error) {
	manifestData, err := containerdutil.ReadBlob(ctx, provider, desc)
	if err != nil {
		return nil, err
	}
	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, err
	}

	configData, err := containerdutil.ReadBlob(ctx, provider, manifest.Config)
	if err != nil {
		return nil, err
	}
	var config ocispec.Image
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// snapshotUsage returns the size of a single snapshot, or 0 when the image is not unpacked.
func snapshotUsage(ctx context.Context, snapshotter snapshots.Snapshotter, chainID digest.Digest) int64 {
	usage, err := snapshotter.Usage(ctx, chainID.String())
	if err != nil {
		if !errdefs.IsNotFound(err) {
			log.G(ctx).WithError(err).Debugf("failed to get the usage of snapshot %q", chainID)
		}
		return 0
	}
	return usage.Size
}

// uniqueByTarget collapses the names pointing at the same target into a single image, preferring a
// tagged name so that the verbose output shows something more useful than "<none> <none>".
func uniqueByTarget(imageList []images.Image) []images.Image {
	var (
		unique = make([]images.Image, 0, len(imageList))
		index  = map[digest.Digest]int{}
	)
	for _, img := range imageList {
		i, ok := index[img.Target.Digest]
		if !ok {
			index[img.Target.Digest] = len(unique)
			unique = append(unique, img)
			continue
		}
		if _, tag := imgutil.ParseRepoTag(unique[i].Name); tag == "" {
			if _, tag := imgutil.ParseRepoTag(img.Name); tag != "" {
				unique[i] = img
			}
		}
	}
	return unique
}
