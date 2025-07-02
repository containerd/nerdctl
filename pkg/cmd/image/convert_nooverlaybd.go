//go:build no_overlaybd

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
	"fmt"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/images/converter"
	"github.com/containerd/errdefs"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
)

// Define a dummy type to avoid import issues
type Option struct{}

var ErrOverlayBDNotImplemented = fmt.Errorf("%w: overlaybd is disabled by the distributor of this build", errdefs.ErrNotImplemented)

func getOBDConvertOpts(options types.ImageConvertOptions) ([]Option, error) {
	return nil, ErrOverlayBDNotImplemented
}

func addOverlayBDConverterOpts(ctx context.Context, client *containerd.Client, srcRef string, options types.ImageConvertOptions, convertOpts []converter.Opt) ([]converter.Opt, error) {
	return convertOpts, ErrOverlayBDNotImplemented
}