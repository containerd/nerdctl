//go:build no_obd

package converter

import (
	"github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/images/converter"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/features"
)

func OverlayBDConvertOpt(_ types.OverlaybdOptions, _ *client.Client, _ string) (converter.Opt, error) {
	return nil, features.ErrOverlayBDSupportMissing
}
