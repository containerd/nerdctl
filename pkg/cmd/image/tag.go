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

	"github.com/containerd/containerd"
	"github.com/containerd/errdefs"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/idutil/imagewalker"
	"github.com/containerd/nerdctl/pkg/referenceutil"
)

func Tag(ctx context.Context, client *containerd.Client, options types.ImageTagOptions) error {
	imageService := client.ImageService()
	var srcName string
	imagewalker := &imagewalker.ImageWalker{
		Client: client,
		OnFound: func(ctx context.Context, found imagewalker.Found) error {
			if srcName == "" {
				srcName = found.Image.Name
			}
			return nil
		},
	}
	matchCount, err := imagewalker.Walk(ctx, options.Source)
	if err != nil {
		return err
	}
	if matchCount < 1 {
		return fmt.Errorf("%s: not found", options.Source)
	}

	target, err := referenceutil.ParseDockerRef(options.Target)
	if err != nil {
		return err
	}

	ctx, done, err := client.WithLease(ctx)
	if err != nil {
		return err
	}
	defer done(ctx)

	image, err := imageService.Get(ctx, srcName)
	if err != nil {
		return err
	}
	image.Name = target.String()
	if _, err = imageService.Create(ctx, image); err != nil {
		if errdefs.IsAlreadyExists(err) {
			if err = imageService.Delete(ctx, image.Name); err != nil {
				return err
			}
			if _, err = imageService.Create(ctx, image); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	return nil
}
