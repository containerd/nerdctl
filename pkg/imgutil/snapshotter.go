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
	"strings"

	socisource "github.com/awslabs/soci-snapshotter/fs/source"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/images"
	ctdsnapshotters "github.com/containerd/containerd/pkg/snapshotters"
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
