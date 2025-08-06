//go:build no_ipfs

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

package ipfs

import (
	"context"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/images/converter"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/features"
	"github.com/containerd/nerdctl/v2/pkg/imgutil"
)

// EnsureImage pull the specified image from IPFS.
func EnsureImage(ctx context.Context, client *containerd.Client, scheme, ref, ipfsPath string, options types.ImagePullOptions) (*imgutil.EnsuredImage, error) {
	return nil, features.ErrIPFSSupportMissing
}

// Push pushes the specified image to IPFS.
func Push(ctx context.Context, client *containerd.Client, rawRef string, layerConvert converter.ConvertFunc, allPlatforms bool, platform []string, ensureImage bool, ipfsPath string) (string, error) {
	return "", features.ErrIPFSSupportMissing
}
