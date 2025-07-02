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

package commit

import (
	"context"
	"fmt"

	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/log"
	"github.com/containerd/stargz-snapshotter/estargz"
	estargzconvert "github.com/containerd/stargz-snapshotter/nativeconverter/estargz"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
)

func convertToEstargz(ctx context.Context, cs content.Store, newDesc ocispec.Descriptor, mediaType string, opts types.EstargzOptions) (*ocispec.Descriptor, error) {
	if !opts.Estargz {
		return nil, nil
	}

	log.G(ctx).Infof("Converting diff layer to eStargz format")

	esgzOpts := []estargz.Option{
		estargz.WithCompressionLevel(opts.EstargzCompressionLevel),
	}
	if opts.EstargzChunkSize > 0 {
		esgzOpts = append(esgzOpts, estargz.WithChunkSize(opts.EstargzChunkSize))
	}
	if opts.EstargzMinChunkSize > 0 {
		esgzOpts = append(esgzOpts, estargz.WithMinChunkSize(opts.EstargzMinChunkSize))
	}

	convertFunc := estargzconvert.LayerConvertFunc(esgzOpts...)

	esgzDesc, err := convertFunc(ctx, cs, newDesc)
	if err != nil {
		return nil, fmt.Errorf("failed to convert diff layer to eStargz: %w", err)
	} else if esgzDesc != nil {
		esgzDesc.MediaType = mediaType
		esgzInfo, err := cs.Info(ctx, esgzDesc.Digest)
		if err != nil {
			return nil, err
		}
		if esgzInfo.Labels == nil {
			esgzInfo.Labels = make(map[string]string)
		}
		esgzInfo.Labels["containerd.io/uncompressed"] = newDesc.Digest.String()
		if _, err := cs.Update(ctx, esgzInfo); err != nil {
			return nil, err
		}
		return esgzDesc, nil
	}

	return nil, nil
}