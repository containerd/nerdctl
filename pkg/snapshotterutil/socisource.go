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

/*
   Copyright The Soci Snapshotter Authors.

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

// Taken from https://github.com/awslabs/soci-snapshotter/blob/237fc956b8366e49927c84fcfee9a2defbb8f53c/fs/source/source.go
// to avoid taking dependency, as maintainers do not wish to upgrade to containerd v2 yet.

package snapshotterutil

import (
	"context"
	"fmt"
	"strings"

	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/labels"
	ctdsnapshotters "github.com/containerd/containerd/pkg/snapshotters"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	// TargetSizeLabel is a label which contains layer size.
	TargetSizeLabel = "containerd.io/snapshot/remote/soci.size"

	// targetImageLayersSizeLabel is a label which contains layer sizes contained in
	// the target image.
	targetImageLayersSizeLabel = "containerd.io/snapshot/remote/image.layers.size"

	// TargetSociIndexDigestLabel is a label which contains the digest of the soci index.
	TargetSociIndexDigestLabel = "containerd.io/snapshot/remote/soci.index.digest"
)

// SociAppendDefaultLabelsHandlerWrapper makes a handler which appends image's basic
// information to each layer descriptor as annotations during unpack. These
// annotations will be passed to this remote snapshotter as labels and used to
// construct source information.
func SociAppendDefaultLabelsHandlerWrapper(indexDigest string, wrapper func(images.Handler) images.Handler) func(f images.Handler) images.Handler {
	return func(f images.Handler) images.Handler {
		return images.HandlerFunc(func(ctx context.Context, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
			children, err := wrapper(f).Handle(ctx, desc)
			if err != nil {
				return nil, err
			}
			switch desc.MediaType {
			case ocispec.MediaTypeImageManifest, images.MediaTypeDockerSchema2Manifest:
				for i := range children {
					c := &children[i]
					if images.IsLayerType(c.MediaType) {
						if c.Annotations == nil {
							c.Annotations = make(map[string]string)
						}

						c.Annotations[TargetSizeLabel] = fmt.Sprintf("%d", c.Size)
						c.Annotations[TargetSociIndexDigestLabel] = indexDigest

						remainingLayerDigestsCount := len(strings.Split(c.Annotations[ctdsnapshotters.TargetImageLayersLabel], ","))

						var layerSizes string
						/*
							We must ensure that the counts of layer sizes and layer digests are equal.
							We will limit the # of neighboring label sizes to equal the # of neighboring
							layer digests for any given layer.
						*/
						for _, l := range children[i : i+remainingLayerDigestsCount] {
							if images.IsLayerType(l.MediaType) {
								ls := fmt.Sprintf("%d,", l.Size)
								// This avoids the label hits the size limitation.
								// Skipping layers is allowed here and only affects performance.
								if err := labels.Validate(targetImageLayersSizeLabel, layerSizes+ls); err != nil {
									break
								}
								layerSizes += ls
							}
						}
						c.Annotations[targetImageLayersSizeLabel] = strings.TrimSuffix(layerSizes, ",")
					}
				}
			}
			return children, nil
		})
	}
}
