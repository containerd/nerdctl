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

package containerdutil

import (
	"context"

	"github.com/containerd/containerd/v2/core/content"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

var ReadBlob = readBlobWithCache()

type readBlob func(ctx context.Context, provider content.Provider, desc ocispec.Descriptor) ([]byte, error)

func readBlobWithCache() readBlob {
	var cache = make(map[string]([]byte))

	return func(ctx context.Context, provider content.Provider, desc ocispec.Descriptor) ([]byte, error) {
		var err error
		v, ok := cache[desc.Digest.String()]
		if !ok {
			v, err = content.ReadBlob(ctx, provider, desc)
			if err == nil {
				cache[desc.Digest.String()] = v
			}
		}

		return v, err
	}
}
