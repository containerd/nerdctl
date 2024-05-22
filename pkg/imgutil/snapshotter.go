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
	"strings"

	socisource "github.com/awslabs/soci-snapshotter/fs/source"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/mount"
	ctdsnapshotters "github.com/containerd/containerd/pkg/snapshotters"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/imgutil/pull"
	"github.com/containerd/stargz-snapshotter/fs/source"
)

const (
	snapshotterNameOverlaybd = "overlaybd"
	snapshotterNameStargz    = "stargz"
	snapshotterNameNydus     = "nydus"
	snapshotterNameSoci      = "soci"
	snapshotterNameCvmfs     = "cvmfs-snapshotter"

	// prefetch size for stargz
	prefetchSize = 10 * 1024 * 1024
)

// remote snapshotters explicitly handled by nerdctl
var builtinRemoteSnapshotterOpts = map[string]snapshotterOpts{
	snapshotterNameOverlaybd: &remoteSnapshotterOpts{snapshotter: "overlaybd"},
	snapshotterNameStargz:    &remoteSnapshotterOpts{snapshotter: "stargz", extraLabels: stargzExtraLabels},
	snapshotterNameNydus:     &remoteSnapshotterOpts{snapshotter: "nydus"},
	snapshotterNameSoci:      &remoteSnapshotterOpts{snapshotter: "soci", extraLabels: sociExtraLabels},
	snapshotterNameCvmfs:     &remoteSnapshotterOpts{snapshotter: "cvmfs-snapshotter"},
}

// snapshotterOpts is used to update pull config
// for different snapshotters
type snapshotterOpts interface {
	apply(config *pull.Config, ref string, rFlags types.RemoteSnapshotterFlags)
	isRemote() bool
}

// getSnapshotterOpts get snapshotter opts by fuzzy matching of the snapshotter name
func getSnapshotterOpts(snapshotter string) snapshotterOpts {
	for sn, sno := range builtinRemoteSnapshotterOpts {
		if strings.Contains(snapshotter, sn) {
			if snapshotter != sn {
				log.L.Debugf("assuming %s to be a %s-compatible snapshotter", snapshotter, sn)
			}
			return sno
		}
	}

	return &defaultSnapshotterOpts{snapshotter: snapshotter}
}

// remoteSnapshotterOpts is used as a remote snapshotter implementation for
// interface `snapshotterOpts.isRemote()` function
type remoteSnapshotterOpts struct {
	snapshotter string
	extraLabels func(func(images.Handler) images.Handler, types.RemoteSnapshotterFlags) func(images.Handler) images.Handler
}

func (rs *remoteSnapshotterOpts) isRemote() bool {
	return true
}

func (rs *remoteSnapshotterOpts) apply(config *pull.Config, ref string, rFlags types.RemoteSnapshotterFlags) {
	h := ctdsnapshotters.AppendInfoHandlerWrapper(ref)
	if rs.extraLabels != nil {
		h = rs.extraLabels(h, rFlags)
	}
	config.RemoteOpts = append(
		config.RemoteOpts,
		containerd.WithImageHandlerWrapper(h),
		containerd.WithPullSnapshotter(rs.snapshotter),
	)
}

// defaultSnapshotterOpts is for snapshotters that
// not handled separately
type defaultSnapshotterOpts struct {
	snapshotter string
}

func (dsn *defaultSnapshotterOpts) apply(config *pull.Config, _ref string, rFlags types.RemoteSnapshotterFlags) {
	config.RemoteOpts = append(
		config.RemoteOpts,
		containerd.WithPullSnapshotter(dsn.snapshotter))
}

// defaultSnapshotterOpts is not a remote snapshotter
func (dsn *defaultSnapshotterOpts) isRemote() bool {
	return false
}

func stargzExtraLabels(f func(images.Handler) images.Handler, rFlags types.RemoteSnapshotterFlags) func(images.Handler) images.Handler {
	return source.AppendExtraLabelsHandler(prefetchSize, f)
}

func sociExtraLabels(f func(images.Handler) images.Handler, rFlags types.RemoteSnapshotterFlags) func(images.Handler) images.Handler {
	return socisource.AppendDefaultLabelsHandlerWrapper(rFlags.SociIndexDigest, f)
}

func SnapshotServiceWithCache(nativeSnapshotter snapshots.Snapshotter) snapshots.Snapshotter {
	return &CachingSnapshotter{
		containerdSnapshotter: nativeSnapshotter,
		statCache:             map[string]snapshots.Info{},
		usageCache:            map[string]snapshots.Usage{},
	}
}

type CachingSnapshotter struct {
	containerdSnapshotter snapshots.Snapshotter
	statCache             map[string]snapshots.Info
	usageCache            map[string]snapshots.Usage
}

func (snap *CachingSnapshotter) Stat(ctx context.Context, key string) (snapshots.Info, error) {
	if stat, ok := snap.statCache[key]; ok {
		return stat, nil
	}
	stat, err := snap.containerdSnapshotter.Stat(ctx, key)
	if err == nil {
		snap.statCache[key] = stat
	}
	return stat, err
}

func (snap *CachingSnapshotter) Update(ctx context.Context, info snapshots.Info, fieldpaths ...string) (snapshots.Info, error) {
	return snap.containerdSnapshotter.Update(ctx, info, fieldpaths...)
}

func (snap *CachingSnapshotter) Usage(ctx context.Context, key string) (snapshots.Usage, error) {
	if usage, ok := snap.usageCache[key]; ok {
		return usage, nil
	}
	usage, err := snap.containerdSnapshotter.Usage(ctx, key)
	if err == nil {
		snap.usageCache[key] = usage
	}
	return usage, err
}

func (snap *CachingSnapshotter) Mounts(ctx context.Context, key string) ([]mount.Mount, error) {
	return snap.containerdSnapshotter.Mounts(ctx, key)
}

func (snap *CachingSnapshotter) Prepare(ctx context.Context, key, parent string, opts ...snapshots.Opt) ([]mount.Mount, error) {
	return snap.containerdSnapshotter.Prepare(ctx, key, parent, opts...)
}

func (snap *CachingSnapshotter) View(ctx context.Context, key, parent string, opts ...snapshots.Opt) ([]mount.Mount, error) {
	return snap.containerdSnapshotter.View(ctx, key, parent, opts...)
}

func (snap *CachingSnapshotter) Commit(ctx context.Context, name, key string, opts ...snapshots.Opt) error {
	return snap.containerdSnapshotter.Commit(ctx, name, key, opts...)
}

func (snap *CachingSnapshotter) Remove(ctx context.Context, key string) error {
	return snap.containerdSnapshotter.Remove(ctx, key)
}

func (snap *CachingSnapshotter) Walk(ctx context.Context, fn snapshots.WalkFunc, filters ...string) error {
	return snap.containerdSnapshotter.Walk(ctx, fn, filters...)
}

func (snap *CachingSnapshotter) Close() error {
	return snap.containerdSnapshotter.Close()
}
