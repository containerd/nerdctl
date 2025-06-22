//go:build no_nydus

package converter

import (
	"github.com/containerd/containerd/v2/core/images/converter"
	"github.com/containerd/platforms"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/features"
)

func NydusConvertOpt(_ types.NydusOptions, _ platforms.MatchComparer, _ string) (converter.Opt, error) {
	return nil, features.ErrNydusSupportMissing
}
