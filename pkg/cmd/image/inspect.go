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
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/idutil/imagewalker"
	"github.com/containerd/nerdctl/pkg/imageinspector"
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
)

func Inspect(ctx context.Context, imageFilter []string, options types.ImageInspectOptions) error {
	var clientOpts []containerd.ClientOpt
	if options.Platform != "" {
		platformParsed, err := platforms.Parse(options.Platform)
		if err != nil {
			return err
		}
		platformM := platforms.Only(platformParsed)
		clientOpts = append(clientOpts, containerd.WithDefaultPlatform(platformM))
	}
	client, ctx, cancel, err := clientutil.NewClient(ctx, options.GOptions.Namespace, options.GOptions.Address, clientOpts...)
	if err != nil {
		return err
	}
	defer cancel()

	f := &imageInspector{
		mode: options.Mode,
	}
	walker := &imagewalker.ImageWalker{
		Client: client,
		OnFound: func(ctx context.Context, found imagewalker.Found) error {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			n, err := imageinspector.Inspect(ctx, client, found.Image)
			if err != nil {
				return err
			}
			switch f.mode {
			case "native":
				f.entries = append(f.entries, n)
			case "dockercompat":
				d, err := dockercompat.ImageFromNative(n)
				if err != nil {
					return err
				}
				f.entries = append(f.entries, d)
			default:
				return fmt.Errorf("unknown mode %q", f.mode)
			}
			return nil
		},
	}

	var errs []error
	for _, req := range imageFilter {
		n, err := walker.Walk(ctx, req)
		if err != nil {
			errs = append(errs, err)
		} else if n == 0 {
			errs = append(errs, fmt.Errorf("no such object: %s", req))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d errors: %v", len(errs), errs)
	}
	return formatter.FormatSlice(options.Format, options.Stdout, f.entries)
}

type imageInspector struct {
	mode    string
	entries []interface{}
}
