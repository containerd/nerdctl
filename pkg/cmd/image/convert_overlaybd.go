//go:build !no_overlaybd

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

package image

import (
	"context"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/images/converter"
	overlaybdconvert "github.com/containerd/accelerated-container-image/pkg/convertor"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
)

func getOBDConvertOpts(options types.ImageConvertOptions) ([]overlaybdconvert.Option, error) {
	obdOpts := []overlaybdconvert.Option{
		overlaybdconvert.WithFsType(options.OverlayFsType),
		overlaybdconvert.WithDbstr(options.OverlaydbDBStr),
	}
	return obdOpts, nil
}

func addOverlayBDConverterOpts(ctx context.Context, client *containerd.Client, srcRef string, options types.ImageConvertOptions, convertOpts []converter.Opt) ([]converter.Opt, error) {
	obdOpts, err := getOBDConvertOpts(options)
	if err != nil {
		return convertOpts, err
	}
	obdOpts = append(obdOpts, overlaybdconvert.WithClient(client))
	obdOpts = append(obdOpts, overlaybdconvert.WithImageRef(srcRef))
	convertFunc := overlaybdconvert.IndexConvertFunc(obdOpts...)
	convertOpts = append(convertOpts, converter.WithIndexConvertFunc(convertFunc))
	return convertOpts, nil
}