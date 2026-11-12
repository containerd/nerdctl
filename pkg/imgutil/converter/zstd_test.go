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

package converter

import (
	"context"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"gotest.tools/v3/assert"

	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/plugins/content/local"
	"github.com/containerd/errdefs"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
)

func TestZstdLayerConvertFuncSkipsZstdLayers(t *testing.T) {
	convertFunc, err := ZstdLayerConvertFunc(types.ImageConvertOptions{
		ZstdOptions: types.ZstdOptions{Zstd: true, ZstdCompressionLevel: 3},
	})
	assert.NilError(t, err)

	// The store is empty: a layer that is converted fails to be read,
	// while a layer that is skipped returns before being read.
	cs, err := local.NewStore(t.TempDir())
	assert.NilError(t, err)

	testCases := []struct {
		name      string
		mediaType string
		skipped   bool
	}{
		{name: "OCIZstd", mediaType: ocispec.MediaTypeImageLayerZstd, skipped: true},
		{name: "OCIGzip", mediaType: ocispec.MediaTypeImageLayerGzip},
		{name: "OCIUncompressed", mediaType: ocispec.MediaTypeImageLayer},
		{name: "DockerGzip", mediaType: images.MediaTypeDockerSchema2LayerGzip},
		{name: "DockerZstd", mediaType: images.MediaTypeDockerSchema2LayerZstd},
		{name: "Config", mediaType: ocispec.MediaTypeImageConfig, skipped: true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			desc := ocispec.Descriptor{
				MediaType: tc.mediaType,
				Digest:    digest.FromString(tc.name),
				Size:      1,
			}
			newDesc, err := convertFunc(context.Background(), cs, desc)
			if tc.skipped {
				assert.NilError(t, err)
				assert.Assert(t, newDesc == nil)
			} else {
				assert.Assert(t, errdefs.IsNotFound(err), "expected the layer to be read, got %v", err)
			}
		})
	}
}
