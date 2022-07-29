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

package imgutil

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/nerdctl/pkg/imgutil/pull"
	nyduslabel "github.com/containerd/nydus-snapshotter/pkg/label"
	"github.com/containerd/stargz-snapshotter/fs/config"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"gotest.tools/v3/assert"
)

const (
	targetRefLabel = "containerd.io/snapshot/remote/stargz.reference"
	testRef        = "test:latest"
)

func TestGetSnapshotterOpts(t *testing.T) {
	type testCase struct {
		sns  []string
		want snapshotterOpts
	}
	testCases := []testCase{
		{
			sns:  []string{"overlayfs"},
			want: &defaultSnapshotterOpts{snapshotter: "overlayfs"},
		},
		{
			sns:  []string{"overlayfs2"},
			want: &defaultSnapshotterOpts{snapshotter: "overlayfs2"},
		},
		{
			sns:  []string{"stargz", "stargz-v1"},
			want: &stargzSnapshotterOpts{},
		},
		{
			sns:  []string{"overlaybd", "overlaybd-v2"},
			want: &overlaybdSnapshotterOpts{},
		},
		{
			sns:  []string{"nydus", "nydus-v3"},
			want: &nydusSnapshotterOpts{},
		},
	}
	for _, tc := range testCases {
		for i := range tc.sns {
			got := getSnapshotterOpts(tc.sns[i])
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("getSnapshotterOpts() got = %v, want %v", got, tc.want)
			}
		}
	}
}

func getAndApplyRemoteOpts(t *testing.T, sn string) *containerd.RemoteContext {
	config := &pull.Config{}
	snOpts := getSnapshotterOpts(sn)
	snOpts.apply(config, testRef)

	rc := &containerd.RemoteContext{}
	for _, o := range config.RemoteOpts {
		// here passing a nil client is safe
		// because the remote opts will not use client
		if err := o(nil, rc); err != nil {
			t.Errorf("failed to apply remote opts: %s", err)
		}
	}

	return rc
}

func TestDefaultSnapshotterOpts(t *testing.T) {
	rc := getAndApplyRemoteOpts(t, "overlayfs")
	assert.Equal(t, rc.Snapshotter, "overlayfs")
}

// dummyImageHandler will return a dummy layer
// see https://github.com/containerd/containerd/blob/77d53d2d230c3bcd3f02e6f493019a72905c875b/images/mediatypes.go#L115
type dummyImageHandler struct{}

func (dih *dummyImageHandler) Handle(_ctx context.Context, _desc ocispec.Descriptor) (subdescs []ocispec.Descriptor, err error) {
	return []ocispec.Descriptor{
		{
			MediaType: "application/vnd.oci.image.layer.dummy",
		},
	}, nil
}

func TestNydusSnapshotterOpts(t *testing.T) {
	rc := getAndApplyRemoteOpts(t, "nydus")
	assert.Equal(t, rc.Snapshotter, snapshotterNameNydus)

	desc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
	}

	h := &dummyImageHandler{}
	got, err := rc.HandlerWrapper(h).Handle(context.Background(), desc)

	assert.NilError(t, err)
	assert.Check(t, len(got) == 1)
	assert.Check(t, got[0].Annotations != nil)
	assert.Equal(t, got[0].Annotations[nyduslabel.CRIImageRef], testRef)
}

func TestOverlaybdSnapshotterOpts(t *testing.T) {
	rc := getAndApplyRemoteOpts(t, "overlaybd")
	assert.Equal(t, rc.Snapshotter, snapshotterNameOverlaybd)

	info := &snapshots.Info{}
	assert.Check(t, rc.SnapshotterOpts != nil)

	for _, o := range rc.SnapshotterOpts {
		err := o(info)
		assert.NilError(t, err)
	}

	assert.Check(t, info != nil)
	assert.Check(t, info.Labels != nil)
	assert.Check(t, len(info.Labels) == 1)

	assert.Equal(t, info.Labels[overlaybdLabelImageRef], testRef)
}

func TestStargzSnapshotterOpts(t *testing.T) {
	rc := getAndApplyRemoteOpts(t, "stargz")
	assert.Equal(t, rc.Snapshotter, snapshotterNameStargz)

	desc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
	}

	h := &dummyImageHandler{}
	got, err := rc.HandlerWrapper(h).Handle(context.Background(), desc)

	assert.NilError(t, err)
	assert.Check(t, len(got) == 1)
	assert.Check(t, got[0].Annotations != nil)
	assert.Equal(t, got[0].Annotations[config.TargetPrefetchSizeLabel], fmt.Sprintf("%d", prefetchSize))
	assert.Equal(t, got[0].Annotations[targetRefLabel], testRef)
}
