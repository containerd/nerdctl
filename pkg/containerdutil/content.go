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

// Package containerdutil provides "caching" versions of containerd native snapshotter and content store.
// NOTE: caching should only be used for single, atomic operations, like `nerdctl images`, and NOT kept
// across successive, distincts operations. As such, caching is not persistent across invocations of nerdctl,
// and only lasts as long as the lifetime of the Snapshotter or ContentStore.
package containerdutil

import (
	"context"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/content"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// ContentStore should be called to get a Provider with caching
func NewProvider(client *containerd.Client) content.Provider {
	return &providerWithCache{
		client.ContentStore(),
		make(map[string]*readerAtWithCache),
	}
}

type providerWithCache struct {
	native content.Provider
	cache  map[string]*readerAtWithCache
}

func (provider *providerWithCache) ReaderAt(ctx context.Context, desc ocispec.Descriptor) (content.ReaderAt, error) {
	key := desc.Digest.String()
	// If we had en entry already, get the size over
	value, ok := provider.cache[key]
	if !ok {
		newReaderAt, err := provider.native.ReaderAt(ctx, desc)
		if err != nil {
			return nil, err
		}
		// Build the final object
		value = &readerAtWithCache{
			newReaderAt,
			-1,
			func() {
				delete(provider.cache, key)
			},
		}
		// Cache it
		provider.cache[key] = value
	}

	return value, nil
}

// ReaderAtWithCache implements the content.ReaderAt interface
type readerAtWithCache struct {
	content.ReaderAt
	size  int64
	prune func()
}

func (rac *readerAtWithCache) Size() int64 {
	// local implementation in containerd technically provides a similar mechanism, so, this method not really useful
	// by default - but obviously, this is implementation dependent
	if rac.size == -1 {
		rac.size = rac.ReaderAt.Size()
	}
	return rac.size
}

func (rac *readerAtWithCache) Close() error {
	err := rac.ReaderAt.Close()
	// Remove ourselves from the cache
	rac.prune()
	return err
}
