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

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/nerdctl/pkg/imgutil/pull"
	nyduslabel "github.com/containerd/nydus-snapshotter/pkg/label"
	"github.com/containerd/stargz-snapshotter/fs/source"
	"github.com/sirupsen/logrus"
)

const (
	snapshotterNameOverlaybd = "overlaybd"
	snapshotterNameStargz    = "stargz"
	snapshotterNameNydus     = "nydus"

	// prefetch size for stargz
	prefetchSize = 10 * 1024 * 1024

	overlaybdLabelImageRef = "containerd.io/snapshot/image-ref"
)

// remote snapshotters explicitly handled by nerdctl
var builtinRemoteSnapshotterOpts = map[string]snapshotterOpts{
	snapshotterNameOverlaybd: &overlaybdSnapshotterOpts{},
	snapshotterNameStargz:    &stargzSnapshotterOpts{},
	snapshotterNameNydus:     &nydusSnapshotterOpts{},
}

// snapshotterOpts is used to update pull config
// for different snapshotters
type snapshotterOpts interface {
	apply(config *pull.Config, ref string)
	isRemote() bool
}

// getSnapshotterOpts get snapshotter opts by fuzzy matching of the snapshotter name
func getSnapshotterOpts(snapshotter string) snapshotterOpts {
	for sn, sno := range builtinRemoteSnapshotterOpts {
		if strings.Contains(snapshotter, sn) {
			if snapshotter != sn {
				logrus.Debugf("assuming %s to be a %s-compatible snapshotter", snapshotter, sn)
			}
			return sno
		}
	}

	return &defaultSnapshotterOpts{snapshotter: snapshotter}
}

// remoteSnapshotter is used as a default implementation for
// interface `snapshotterOpts.isRemote()` function
type remoteSnapshotter struct{}

func (rs *remoteSnapshotter) isRemote() bool {
	return true
}

// defaultSnapshotterOpts is for snapshotters that
// not handled separately
type defaultSnapshotterOpts struct {
	snapshotter string
}

func (dsn *defaultSnapshotterOpts) apply(config *pull.Config, _ref string) {
	config.RemoteOpts = append(
		config.RemoteOpts,
		containerd.WithPullSnapshotter(dsn.snapshotter))
}

// defaultSnapshotterOpts is not a remote snapshotter
func (dsn *defaultSnapshotterOpts) isRemote() bool {
	return false
}

// stargzSnapshotterOpts for stargz snapshotter
type stargzSnapshotterOpts struct {
	remoteSnapshotter
}

func (ssn *stargzSnapshotterOpts) apply(config *pull.Config, ref string) {
	// TODO: support "skip-content-verify"
	config.RemoteOpts = append(
		config.RemoteOpts,
		containerd.WithImageHandlerWrapper(source.AppendDefaultLabelsHandlerWrapper(ref, prefetchSize)),
		containerd.WithPullSnapshotter(snapshotterNameStargz),
	)
}

// overlaybdSnapshotterOpts for overlaybd snapshotter
type overlaybdSnapshotterOpts struct {
	remoteSnapshotter
}

func (osn *overlaybdSnapshotterOpts) apply(config *pull.Config, ref string) {
	snlabel := map[string]string{overlaybdLabelImageRef: ref}
	logrus.Debugf("append remote opts: %s", snlabel)

	config.RemoteOpts = append(
		config.RemoteOpts,
		containerd.WithPullSnapshotter(snapshotterNameOverlaybd, snapshots.WithLabels(snlabel)),
	)
}

// nydusSnapshotterOpts for nydus snapshotter
type nydusSnapshotterOpts struct {
	remoteSnapshotter
}

func (nsn *nydusSnapshotterOpts) apply(config *pull.Config, ref string) {
	config.RemoteOpts = append(
		config.RemoteOpts,
		containerd.WithImageHandlerWrapper(nyduslabel.AppendLabelsHandlerWrapper(ref)),
		containerd.WithPullSnapshotter(snapshotterNameNydus),
	)
}
