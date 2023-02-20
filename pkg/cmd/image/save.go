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
	"github.com/containerd/containerd/images/archive"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/idutil/imagewalker"
	"github.com/containerd/nerdctl/pkg/platformutil"
	"github.com/containerd/nerdctl/pkg/strutil"
)

// Save exports `images` to a `io.Writer` (e.g., a file writer, or os.Stdout) specified by `options.Stdout`.
func Save(ctx context.Context, client *containerd.Client, images []string, options types.ImageSaveOptions, exportOpts ...archive.ExportOpt) error {
	images = strutil.DedupeStrSlice(images)

	platMC, err := platformutil.NewMatchComparer(options.AllPlatforms, options.Platform)
	if err != nil {
		return err
	}

	exportOpts = append(exportOpts, archive.WithPlatform(platMC))
	imageStore := client.ImageService()

	savedImages := make(map[string]struct{})
	walker := &imagewalker.ImageWalker{
		Client: client,
		OnFound: func(ctx context.Context, found imagewalker.Found) error {
			if found.UniqueImages > 1 {
				return fmt.Errorf("ambiguous digest ID: multiple IDs found with provided prefix %s", found.Req)
			}
			imgName := found.Image.Name
			imgDigest := found.Image.Target.Digest.String()
			if _, ok := savedImages[imgDigest]; !ok {
				savedImages[imgDigest] = struct{}{}
				exportOpts = append(exportOpts, archive.WithImage(imageStore, imgName))
			}
			return nil
		},
	}

	// check if all images exist
	if err := walker.WalkAll(ctx, images, false); err != nil {
		return err
	}

	return client.Export(ctx, options.Stdout, exportOpts...)
}
