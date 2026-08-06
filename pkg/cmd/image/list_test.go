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
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/containerd/v2/core/images"

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
