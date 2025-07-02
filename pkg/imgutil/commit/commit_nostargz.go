//go:build no_stargz

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

package commit

import (
	"context"
	"fmt"

	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/errdefs"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
)

var ErrStargzNotImplemented = fmt.Errorf("%w: stargz is disabled by the distributor of this build", errdefs.ErrNotImplemented)

func convertToEstargz(ctx context.Context, cs content.Store, newDesc ocispec.Descriptor, mediaType string, opts types.EstargzOptions) (*ocispec.Descriptor, error) {
	if opts.Estargz {
		return nil, ErrStargzNotImplemented
	}
	return nil, nil
}