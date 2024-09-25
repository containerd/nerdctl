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
	"github.com/containerd/errdefs"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/idutil/imagewalker"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
)

func Tag(ctx context.Context, client *containerd.Client, options types.ImageTagOptions) error {
	imageService := client.ImageService()
	var srcName string
	walker := &imagewalker.ImageWalker{
		Client: client,
		OnFound: func(ctx context.Context, found imagewalker.Found) error {
			if srcName == "" {
				srcName = found.Image.Name
			}
			return nil
		},
	}
	matchCount, err := walker.Walk(ctx, options.Source)
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

	// Ensure all the layers are here: https://github.com/containerd/nerdctl/issues/3425
	err = EnsureAllContent(ctx, client, srcName, options.GOptions)
	if err != nil {
		log.G(ctx).Warn("Unable to fetch missing layers before committing. " +
			"If you try to save or push this image, it might fail. See https://github.com/containerd/nerdctl/issues/3439.")
	}

	img, err := imageService.Get(ctx, srcName)
	if err != nil {
		return err
	}

	img.Name = target.String()
	if _, err = imageService.Create(ctx, img); err != nil {
		if errdefs.IsAlreadyExists(err) {
			if err = imageService.Delete(ctx, img.Name); err != nil {
				return err
			}
			if _, err = imageService.Create(ctx, img); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	return nil
}
