//go:build !no_obd

package converter

import (
	"github.com/containerd/accelerated-container-image/pkg/convertor"
	"github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/images/converter"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
)

func OverlayBDConvertOpt(options types.OverlaybdOptions, client *client.Client, srcRef string) (converter.Opt, error) {
	return converter.WithIndexConvertFunc(convertor.IndexConvertFunc(
		convertor.WithFsType(options.OverlayFsType),
		convertor.WithDbstr(options.OverlaydbDBStr),
		convertor.WithClient(client),
		convertor.WithImageRef(srcRef),
	)), nil
}
