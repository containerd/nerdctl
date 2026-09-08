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
	"slices"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"gotest.tools/v3/assert"

	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/platforms"
)

func testDigest(name string) digest.Digest {
	return digest.FromString(name)
}

// newTestContents builds an image whose snapshots are named by chainIDs and whose blobs are the
// given name/size pairs.
func newTestContents(name string, chainIDs []string, blobs map[string]int64) *imageContents {
	contents := &imageContents{
		image: images.Image{
			Name:   name,
			Target: ocispec.Descriptor{Digest: testDigest(name)},
		},
		blobs: map[digest.Digest]int64{},
	}
	for _, chainID := range chainIDs {
		contents.chainIDs = append(contents.chainIDs, testDigest(chainID))
	}
	for blob, size := range blobs {
		contents.blobs[testDigest(blob)] = size
	}
	return contents
}

// snapshotSizes turns a name-keyed table into the usage callback diskUsageIndex.add expects.
func snapshotSizes(sizes map[string]int64) func(digest.Digest) int64 {
	byDigest := map[digest.Digest]int64{}
	for name, size := range sizes {
		byDigest[testDigest(name)] = size
	}
	return func(chainID digest.Digest) int64 {
		return byDigest[chainID]
	}
}

func TestDiskUsageIndexSingleImage(t *testing.T) {
	t.Parallel()

	usage := snapshotSizes(map[string]int64{"layer-a": 100, "layer-b": 200})
	contents := newTestContents("solo", []string{"layer-a", "layer-b"}, map[string]int64{
		"manifest": 5,
		"config":   10,
	})

	index := newDiskUsageIndex()
	index.add(contents, usage)

	size, sharedSize := index.sizes(contents)
	assert.Equal(t, size, int64(315))
	// Nothing is shared when there is only one image.
	assert.Equal(t, sharedSize, int64(0))
	assert.Equal(t, index.total(), int64(315))
}

func TestDiskUsageIndexChargesNoImageForTheIndex(t *testing.T) {
	t.Parallel()

	usage := snapshotSizes(map[string]int64{"layer-a": 100})
	contents := newTestContents("multi", []string{"layer-a"}, map[string]int64{
		"manifest": 5,
		"config":   10,
	})
	contents.indexBlobs = map[digest.Digest]int64{testDigest("index"): 2}

	index := newDiskUsageIndex()
	index.add(contents, usage)

	// The index listing the manifests is on disk, so the total counts it, but Docker sizes an
	// image by walking its manifests and never charges it for the index that lists them.
	size, sharedSize := index.sizes(contents)
	assert.Equal(t, size, int64(115))
	assert.Equal(t, sharedSize, int64(0))
	assert.Equal(t, index.total(), int64(117))
}

func TestDiskUsageIndexSharedLayers(t *testing.T) {
	t.Parallel()

	usage := snapshotSizes(map[string]int64{"base": 1000, "top-a": 30, "top-b": 40})
	// Two images built on the same base layer, sharing the base blob too.
	first := newTestContents("first", []string{"base", "top-a"}, map[string]int64{
		"base-blob":  500,
		"config-a":   7,
		"manifest-a": 3,
	})
	second := newTestContents("second", []string{"base", "top-b"}, map[string]int64{
		"base-blob":  500,
		"config-b":   9,
		"manifest-b": 4,
	})

	index := newDiskUsageIndex()
	index.add(first, usage)
	index.add(second, usage)

	firstSize, firstShared := index.sizes(first)
	assert.Equal(t, firstSize, int64(1000+30+500+7+3))
	assert.Equal(t, firstShared, int64(1000+500))

	secondSize, secondShared := index.sizes(second)
	assert.Equal(t, secondSize, int64(1000+40+500+9+4))
	assert.Equal(t, secondShared, int64(1000+500))

	// The shared base layer and the shared blob are counted once in the total.
	assert.Equal(t, index.total(), int64(1000+30+40+500+7+3+9+4))
	// The total is what is really on disk, so it is less than the sum of the image sizes.
	assert.Assert(t, index.total() < firstSize+secondSize)

	// Only the unique part of an unused image can be reclaimed.
	assert.Equal(t, firstSize-firstShared, int64(30+7+3))
}

func TestDiskUsageIndexNotUnpacked(t *testing.T) {
	t.Parallel()

	// An image that was pulled but never unpacked has no snapshots, so only its content counts.
	usage := snapshotSizes(nil)
	contents := newTestContents("packed", []string{"layer-a"}, map[string]int64{"config": 12})

	index := newDiskUsageIndex()
	index.add(contents, usage)

	size, sharedSize := index.sizes(contents)
	assert.Equal(t, size, int64(12))
	assert.Equal(t, sharedSize, int64(0))
	assert.Equal(t, index.total(), int64(12))
}

func TestDiskUsageIndexMeasuresSnapshotsOnce(t *testing.T) {
	t.Parallel()

	// A snapshot shared by several images must not be measured again for each of them: on a real
	// snapshotter that lookup is a disk walk.
	var calls int
	usage := func(digest.Digest) int64 {
		calls++
		return 100
	}

	index := newDiskUsageIndex()
	index.add(newTestContents("first", []string{"base"}, nil), usage)
	index.add(newTestContents("second", []string{"base"}, nil), usage)

	assert.Equal(t, calls, 1)
	assert.Equal(t, index.total(), int64(100))
}

func TestImageContentsCreatedAt(t *testing.T) {
	t.Parallel()

	built := time.Date(2020, 3, 1, 12, 0, 0, 0, time.UTC)
	pulled := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

	contents := newTestContents("dated", nil, nil)
	contents.image.CreatedAt = pulled

	// Without a config saying otherwise, all we know is when the record appeared locally.
	assert.Equal(t, contents.createdAt(), pulled)

	// The config is authoritative: pulling an old image must not make it look brand new.
	contents.created = &built
	assert.Equal(t, contents.createdAt(), built)
}

// foreignPlatform returns a platform this host does not run. nerdctl is released for linux/s390x
// among others, so no architecture can be hardcoded as the foreign one: a host matches at most one
// of the two candidates below, and a host running neither Linux nor a Linux runtime matches none.
func foreignPlatform(t *testing.T) ocispec.Platform {
	t.Helper()

	matcher := platforms.Default()
	for _, candidate := range []ocispec.Platform{
		{OS: "linux", Architecture: "amd64"},
		{OS: "linux", Architecture: "arm64"},
	} {
		if !matcher.Match(candidate) {
			return candidate
		}
	}
	t.Fatalf("no foreign platform for %q", platforms.Format(platforms.DefaultSpec()))
	return ocispec.Platform{}
}

func TestHostBuildTime(t *testing.T) {
	t.Parallel()

	var (
		host    = platforms.DefaultSpec()
		foreign = foreignPlatform(t)
		older   = time.Date(2020, 3, 1, 0, 0, 0, 0, time.UTC)
		newer   = time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC)
	)

	t.Run("no manifest at all", func(t *testing.T) {
		t.Parallel()
		assert.Assert(t, hostBuildTime(nil) == nil)
	})

	t.Run("the platform of the host wins", func(t *testing.T) {
		t.Parallel()
		// The platforms of an index are not necessarily built together, and the order of the
		// descriptors says nothing, so the host platform must be picked whatever its position.
		built := []buildTime{
			{platform: foreign, created: &older},
			{platform: host, created: &newer},
		}
		assert.Equal(t, *hostBuildTime(built), newer)

		slices.Reverse(built)
		assert.Equal(t, *hostBuildTime(built), newer)
	})

	t.Run("the platform of the host states no build time", func(t *testing.T) {
		t.Parallel()
		// The build time is optional. Once the host platform has answered, the answer stands:
		// reporting the build time of another architecture would be worse than reporting none.
		built := []buildTime{
			{platform: foreign, created: &older},
			{platform: host},
		}
		assert.Assert(t, hostBuildTime(built) == nil)
	})

	t.Run("no platform runs here", func(t *testing.T) {
		t.Parallel()
		// An image pulled for another architecture still describes itself better with a build time
		// than with none, wherever among its platforms that time is stated.
		built := []buildTime{
			{platform: foreign},
			{platform: foreign, created: &older},
		}
		assert.Equal(t, *hostBuildTime(built), older)

		assert.Assert(t, hostBuildTime([]buildTime{{platform: foreign}}) == nil)
	})
}

func TestManifestPlatform(t *testing.T) {
	t.Parallel()

	// alpine ships linux/arm/v6 and linux/arm/v7 manifests whose configs both declare a bare
	// "linux/arm", so the descriptor of the index is the one to believe.
	desc := ocispec.Descriptor{Platform: &ocispec.Platform{OS: "linux", Architecture: "arm", Variant: "v6"}}
	config := &ocispec.Image{Platform: ocispec.Platform{OS: "linux", Architecture: "arm"}}
	assert.Equal(t, platforms.Format(manifestPlatform(desc, config)), "linux/arm/v6")

	// A single-platform image has no index to declare a platform, so the config answers.
	assert.Equal(t, platforms.Format(manifestPlatform(ocispec.Descriptor{}, &ocispec.Image{
		Platform: ocispec.Platform{OS: "linux", Architecture: "amd64"},
	})), "linux/amd64")
}

func TestUniqueByTarget(t *testing.T) {
	t.Parallel()

	shared := ocispec.Descriptor{Digest: testDigest("shared")}
	other := ocispec.Descriptor{Digest: testDigest("other")}

	imageList := []images.Image{
		// The same target under a digest reference, a tag, and a bare config digest, as the k8s.io
		// namespace ends up storing it.
		{Name: "example.com/foo@" + shared.Digest.String(), Target: shared},
		{Name: "example.com/foo:latest", Target: shared},
		{Name: shared.Digest.String(), Target: shared},
		{Name: "example.com/bar:v1", Target: other},
	}

	unique := uniqueByTarget(imageList)
	assert.Equal(t, len(unique), 2)
	// A tagged name is preferred, so the verbose output is not needlessly "<none> <none>".
	assert.Equal(t, unique[0].Name, "example.com/foo:latest")
	assert.Equal(t, unique[1].Name, "example.com/bar:v1")
}

func TestUniqueByTargetKeepsUntagged(t *testing.T) {
	t.Parallel()

	dangling := ocispec.Descriptor{Digest: testDigest("dangling")}
	imageList := []images.Image{
		{Name: "example.com/foo@" + dangling.Digest.String(), Target: dangling},
	}

	unique := uniqueByTarget(imageList)
	assert.Equal(t, len(unique), 1)
	assert.Equal(t, unique[0].Name, imageList[0].Name)
}
