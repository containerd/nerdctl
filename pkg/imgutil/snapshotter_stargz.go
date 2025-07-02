//go:build !no_stargz

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
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/stargz-snapshotter/fs/source"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
)

const (
	snapshotterNameStargz = "stargz"

	// prefetch size for stargz
	prefetchSize = 10 * 1024 * 1024
)

func init() {
	builtinRemoteSnapshotterOpts[snapshotterNameStargz] = &remoteSnapshotterOpts{snapshotter: "stargz", extraLabels: stargzExtraLabels}
}

func stargzExtraLabels(f func(images.Handler) images.Handler, rFlags types.RemoteSnapshotterFlags) func(images.Handler) images.Handler {
	return source.AppendExtraLabelsHandler(prefetchSize, f)
}