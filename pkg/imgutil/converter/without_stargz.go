//go:build no_esgz

package converter

import (
	"context"

	"github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/images/converter"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/features"
)

func ESGZZstdChunkedConvertOpt(_ types.ZstdChunkedOptions, _ bool) (converter.Opt, error) {
	return nil, features.ErrESGZSupportMissing
}

func ESGZConvertOpt(_ types.EstargzOptions, _ bool) (converter.Opt, func(ctx context.Context, cs content.Store, ref string, desc *v1.Descriptor) (*images.Image, error), error) {
	return nil, nil, features.ErrESGZSupportMissing
}
