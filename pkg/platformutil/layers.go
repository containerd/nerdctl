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

package platformutil

import (
	"context"

	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images"
	"github.com/containerd/platforms"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

func LayerDescs(ctx context.Context, provider content.Provider, imageTarget ocispec.Descriptor, platform platforms.MatchComparer) ([]ocispec.Descriptor, error) {
	var descs []ocispec.Descriptor
	err := images.Walk(ctx, images.Handlers(images.HandlerFunc(func(ctx context.Context, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
		if images.IsLayerType(desc.MediaType) {
			descs = append(descs, desc)
		}
		return nil, nil
	}), images.FilterPlatforms(images.ChildrenHandler(provider), platform)), imageTarget)
	return descs, err
}
