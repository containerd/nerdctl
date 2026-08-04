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
	"bytes"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"gotest.tools/v3/assert"

	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/platforms"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/labels"
)

func TestNewViewImageRef(t *testing.T) {
	const digest = "sha256:09538a1f51d3ec5af0449a1640937dfdf79b0e9b8c4da5b8a883086d5c1492ef"
	testCases := []struct {
		name     string
		expected string
	}{
		{"docker.io/library/hello-world:latest", "hello-world:latest"},
		{"docker.io/moby/buildkit:buildx-stable-1", "moby/buildkit:buildx-stable-1"},
		{"ghcr.io/stargz-containers/alpine:3.13", "ghcr.io/stargz-containers/alpine:3.13"},
		// pulled by digest (has an explicit registry domain, no tag) -> repo@digest
		{"docker.io/library/hello-world@" + digest, "hello-world@" + digest},
		{"ghcr.io/stargz-containers/alpine@" + digest, "ghcr.io/stargz-containers/alpine@" + digest},
		// dangling build artifacts: domain-less name with a digest -> untagged
		{"<none>@" + digest, "<untagged>"},
		{"overlayfs@" + digest, "<untagged>"},
		// bare config digest as name (created by the CRI plugin) -> untagged
		{digest, "<untagged>"},
		// unparsable / empty name -> untagged
		{"", "<untagged>"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, newViewImageRef(tc.name), tc.expected)
		})
	}
}

func TestSortByImageRef(t *testing.T) {
	const digest = "sha256:09538a1f51d3ec5af0449a1640937dfdf79b0e9b8c4da5b8a883086d5c1492ef"
	imageList := []images.Image{
		{Name: "<none>@" + digest},
		{Name: "docker.io/library/nginx:alpine"},
		{Name: "ghcr.io/stargz-containers/alpine:3.13"},
		{Name: "overlayfs@" + digest},
		{Name: "docker.io/library/alpine:latest"},
	}
	sortByImageRef(imageList)
	// Ordering is on the rendered IMAGE column (the familiar name), like Docker, so
	// "docker.io/library/nginx:alpine" sorts as "nginx:alpine".
	expected := []string{
		"docker.io/library/alpine:latest",
		"ghcr.io/stargz-containers/alpine:3.13",
		"docker.io/library/nginx:alpine",
		// untagged images come last, in their original order
		"<none>@" + digest,
		"overlayfs@" + digest,
	}
	for i, img := range imageList {
		assert.Equal(t, img.Name, expected[i])
	}
}

func TestPinnedImageDigest(t *testing.T) {
	t.Parallel()

	const pinned = "sha256:09538a1f51d3ec5af0449a1640937dfdf79b0e9b8c4da5b8a883086d5c1492ef"

	testCases := []struct {
		name            string
		containerLabels map[string]string
		expected        string
	}{
		{
			name:            "pinned at creation",
			containerLabels: map[string]string{labels.ImageDigest: pinned},
			expected:        pinned,
		},
		{
			// Containers created before the label existed, or outside nerdctl, have to be resolved
			// by image name instead.
			name:            "no label",
			containerLabels: map[string]string{labels.Platform: "linux/amd64"},
		},
		{
			name:            "empty label",
			containerLabels: map[string]string{labels.ImageDigest: ""},
		},
		{
			// Falling back to the name is better than dropping the container from the in-use set.
			name:            "unparsable label",
			containerLabels: map[string]string{labels.ImageDigest: "not-a-digest"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dgst, ok := pinnedImageDigest(tc.containerLabels)
			assert.Equal(t, ok, tc.expected != "")
			assert.Equal(t, string(dgst), tc.expected)
		})
	}
}

func TestValidateListOptions(t *testing.T) {
	testCases := []struct {
		name     string
		options  types.ImageListOptions
		expected string
	}{
		{
			name:    "no conflict without Tree",
			options: types.ImageListOptions{Quiet: true, Format: "json"},
		},
		{
			name:    "no conflict for Tree alone",
			options: types.ImageListOptions{Tree: true},
		},
		{
			name:     "quiet",
			options:  types.ImageListOptions{Tree: true, Quiet: true},
			expected: "--quiet is not yet supported with --tree",
		},
		{
			name:     "no-trunc",
			options:  types.ImageListOptions{Tree: true, NoTrunc: true},
			expected: "--no-trunc is not yet supported with --tree",
		},
		{
			name:     "digests",
			options:  types.ImageListOptions{Tree: true, Digests: true},
			expected: "--digests is not yet supported with --tree",
		},
		{
			name:     "format",
			options:  types.ImageListOptions{Tree: true, Format: "json"},
			expected: "--format is not yet supported with --tree",
		},
		{
			name:     "names",
			options:  types.ImageListOptions{Tree: true, Names: true},
			expected: "--names is not yet supported with --tree",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateListOptions(&tc.options)
			if tc.expected == "" {
				assert.NilError(t, err)
				return
			}
			assert.Error(t, err, tc.expected)
		})
	}
}

func TestShortImageID(t *testing.T) {
	testCases := []struct {
		name     string
		dgst     digest.Digest
		expected string
	}{
		{
			name:     "digest is truncated to 12 hex characters",
			dgst:     digest.Digest("sha256:" + strings.Repeat("a", 64)),
			expected: strings.Repeat("a", 12),
		},
		{
			name:     "a value without an algorithm is left alone",
			dgst:     digest.Digest("not-a-digest"),
			expected: "not-a-digest",
		},
		{
			name:     "a hex part shorter than 12 characters is left alone",
			dgst:     digest.Digest("sha256:abcd"),
			expected: "sha256:abcd",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, shortImageID(tc.dgst), tc.expected)
		})
	}
}

func TestNormalizePlatform(t *testing.T) {
	// The platforms are keyed by platforms.FormatAll(platforms.Normalize(...)), so an arm64
	// manifest is keyed as a bare "linux/arm64" while a Windows one keeps its OSVersion. A
	// container's label may still carry the arm variant, because platforms.DefaultString does not
	// normalize.
	testCases := []struct {
		platform string
		expected string
	}{
		{"linux/arm64/v8", "linux/arm64"},
		{"linux/arm64/8", "linux/arm64"},
		{"linux/arm64", "linux/arm64"},
		{"linux/amd64", "linux/amd64"},
		{"linux/arm/v7", "linux/arm/v7"},
		{"linux/armhf", "linux/arm/v7"},
		// the OSVersion is what tells two windows/amd64 manifests apart, so it is kept
		{"windows(10.0.20348.2582)/amd64", "windows(10.0.20348.2582)/amd64"},
		{"windows/amd64", "windows/amd64"},
		// an unparsable value is passed through rather than dropped
		{"", ""},
	}
	for _, tc := range testCases {
		t.Run(tc.platform, func(t *testing.T) {
			assert.Equal(t, normalizePlatform(tc.platform), tc.expected)
		})
	}
}

// treeTestImage builds a candidate platform entry with sizes chosen to render exactly at the
// 3-significant-digit precision the Docker v29 views use.
func treeTestImage(os, arch string, dgst digest.Digest, blobSize, snapshotSize int64) *image {
	return &image{
		blobSize:       blobSize,
		size:           snapshotSize,
		platform:       platforms.Platform{OS: os, Architecture: arch},
		manifestDigest: dgst,
		available:      true,
	}
}

func TestPrintImageTree(t *testing.T) {
	const (
		targetDigest = digest.Digest("sha256:" + "1111111111111111111111111111111111111111111111111111111111111111")
		amd64Digest  = digest.Digest("sha256:" + "2222222222222222222222222222222222222222222222222222222222222222")
		arm64Digest  = digest.Digest("sha256:" + "3333333333333333333333333333333333333333333333333333333333333333")
	)
	img := images.Image{
		Name:   "docker.io/library/nginx:latest",
		Target: ocispec.Descriptor{Digest: targetDigest},
	}

	t.Run("multi-platform image expands into a sorted row per platform", func(t *testing.T) {
		var buf bytes.Buffer
		printer := &imagePrinter{
			w:       &buf,
			newView: true,
			tree:    true,
			inUse:   map[digest.Digest]bool{targetDigest: true},
			// Only the amd64 manifest is actually run by a container.
			inUseByPlatform: map[platformRef]bool{{targetDigest, "linux/amd64"}: true},
		}
		// Deliberately insert arm64 first: the candidates come from a map, so the printer has to
		// sort them itself to stay deterministic.
		candidates := map[string]*image{
			"linux/arm64": treeTestImage("linux", "arm64", arm64Digest, 24_000_000, 45_000_000),
			"linux/amd64": treeTestImage("linux", "amd64", amd64Digest, 25_000_000, 47_000_000),
		}

		assert.NilError(t, printer.printImageTree(img, candidates))

		// DISK USAGE is content plus snapshots, CONTENT SIZE is content alone; the parent row
		// aggregates both across the platforms.
		expected := strings.Join([]string{
			"nginx:latest\t111111111111\t141MB\t49MB\tU",
			"├─ linux/amd64\t222222222222\t72MB\t25MB\tU",
			"└─ linux/arm64\t333333333333\t69MB\t24MB\t",
		}, "\n") + "\n"
		assert.Equal(t, buf.String(), expected)
	})

	// Regression test: the displayed name drops the OSVersion, so using it as the identity made a
	// container on one Windows build flag every windows/amd64 row of the index.
	t.Run("windows platforms differing only by OSVersion are told apart", func(t *testing.T) {
		var buf bytes.Buffer
		printer := &imagePrinter{
			w:       &buf,
			newView: true,
			tree:    true,
			inUse:   map[digest.Digest]bool{targetDigest: true},
			// A container runs the older build only.
			inUseByPlatform: map[platformRef]bool{{targetDigest, "windows(10.0.20348.2582)/amd64"}: true},
		}
		candidates := map[string]*image{
			"windows(10.0.26100.1)/amd64": {
				blobSize:       24_000_000,
				size:           45_000_000,
				platform:       platforms.Platform{OS: "windows", Architecture: "amd64", OSVersion: "10.0.26100.1"},
				manifestDigest: arm64Digest,
				available:      true,
			},
			"windows(10.0.20348.2582)/amd64": {
				blobSize:       25_000_000,
				size:           47_000_000,
				platform:       platforms.Platform{OS: "windows", Architecture: "amd64", OSVersion: "10.0.20348.2582"},
				manifestDigest: amd64Digest,
				available:      true,
			},
		}

		assert.NilError(t, printer.printImageTree(img, candidates))

		// Both rows render as windows/amd64, but only the one the container runs is flagged, and
		// the order is stable because the sort uses the full platform.
		expected := strings.Join([]string{
			"nginx:latest\t111111111111\t141MB\t49MB\tU",
			"├─ windows/amd64\t222222222222\t72MB\t25MB\tU",
			"└─ windows/amd64\t333333333333\t69MB\t24MB\t",
		}, "\n") + "\n"
		assert.Equal(t, buf.String(), expected)
	})

	t.Run("a platform listed by the index but never pulled has no size", func(t *testing.T) {
		var buf bytes.Buffer
		printer := &imagePrinter{
			w:               &buf,
			newView:         true,
			tree:            true,
			inUse:           map[digest.Digest]bool{},
			inUseByPlatform: map[platformRef]bool{},
		}
		candidates := map[string]*image{
			"linux/amd64": treeTestImage("linux", "amd64", amd64Digest, 25_000_000, 47_000_000),
			// Listed by the index, but its content is not in the store: no config, no sizes.
			"linux/arm64": {
				platform:       platforms.Platform{OS: "linux", Architecture: "arm64"},
				manifestDigest: arm64Digest,
			},
		}

		assert.NilError(t, printer.printImageTree(img, candidates))

		expected := strings.Join([]string{
			"nginx:latest\t111111111111\t72MB\t25MB\t",
			"├─ linux/amd64\t222222222222\t72MB\t25MB\t",
			"└─ linux/arm64\t333333333333\t0B\t0B\t",
		}, "\n") + "\n"
		assert.Equal(t, buf.String(), expected)
	})

	t.Run("single-platform image gets a single closing branch", func(t *testing.T) {
		var buf bytes.Buffer
		printer := &imagePrinter{
			w:               &buf,
			newView:         true,
			tree:            true,
			inUse:           map[digest.Digest]bool{},
			inUseByPlatform: map[platformRef]bool{},
		}
		candidates := map[string]*image{
			"linux/amd64": treeTestImage("linux", "amd64", targetDigest, 25_000_000, 47_000_000),
		}

		assert.NilError(t, printer.printImageTree(img, candidates))

		expected := strings.Join([]string{
			"nginx:latest\t111111111111\t72MB\t25MB\t",
			"└─ linux/amd64\t111111111111\t72MB\t25MB\t",
		}, "\n") + "\n"
		assert.Equal(t, buf.String(), expected)
	})
}
