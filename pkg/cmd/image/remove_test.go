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
	"testing"

	"github.com/opencontainers/go-digest"
	"gotest.tools/v3/assert"

	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/errdefs"
)

// fakeImageStore is a minimal images.Store that reproduces containerd's real constraint that
// Create fails with errdefs.ErrAlreadyExists when the image name already exists.
type fakeImageStore struct {
	images.Store
	byName map[string]images.Image
}

func newFakeImageStore() *fakeImageStore {
	return &fakeImageStore{byName: map[string]images.Image{}}
}

func (f *fakeImageStore) Create(_ context.Context, image images.Image) (images.Image, error) {
	if _, ok := f.byName[image.Name]; ok {
		return images.Image{}, errdefs.ErrAlreadyExists
	}
	f.byName[image.Name] = image
	return image, nil
}

// TestDanglingImageNameIsUniquePerDigest reproduces
// https://github.com/containerd/nerdctl/issues/4109: force-removing several images that are each
// in use by a running container used to create every kept-alive dangling ref under the exact same
// literal name (":"), so the second `is.Create` call in the same run failed with
// "image \":\": already exists". Naming the dangling ref after its digest (danglingImageName)
// keeps names unique across images, so both creations succeed.
func TestDanglingImageNameIsUniquePerDigest(t *testing.T) {
	store := newFakeImageStore()
	ctx := context.Background()

	digestA := digest.FromString("image-a")
	digestB := digest.FromString("image-b")

	// Old, buggy behavior: every dangling ref reused the same literal ":" name.
	const buggyName = ":"
	_, err := store.Create(ctx, images.Image{Name: buggyName})
	assert.NilError(t, err)
	_, err = store.Create(ctx, images.Image{Name: buggyName})
	assert.Assert(t, errdefs.IsAlreadyExists(err), "expected the second create with a shared name to collide, got %v", err)

	// Fixed behavior: naming the dangling ref after its digest avoids the collision.
	store = newFakeImageStore()
	_, err = store.Create(ctx, images.Image{Name: danglingImageName(digestA)})
	assert.NilError(t, err)
	_, err = store.Create(ctx, images.Image{Name: danglingImageName(digestB)})
	assert.NilError(t, err, "force-removing a second running image must not collide on the dangling ref name")

	assert.Assert(t, danglingImageName(digestA) != danglingImageName(digestB))
}
