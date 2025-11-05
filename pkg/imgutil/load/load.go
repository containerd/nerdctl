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

package load

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/transfer"
	tarchive "github.com/containerd/containerd/v2/core/transfer/archive"
	transferimage "github.com/containerd/containerd/v2/core/transfer/image"
	"github.com/containerd/platforms"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/platformutil"
	"github.com/containerd/nerdctl/v2/pkg/transferutil"
)

// FromArchive loads and unpacks the images from the tar archive specified in image load options.
func FromArchive(ctx context.Context, client *containerd.Client, options types.ImageLoadOptions) ([]images.Image, error) {
	if options.Input != "" {
		f, err := os.Open(options.Input)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		options.Stdin = f
	} else {
		// check if stdin is empty.
		stdinStat, err := os.Stdin.Stat()
		if err != nil {
			return nil, err
		}
		if stdinStat.Size() == 0 && (stdinStat.Mode()&os.ModeNamedPipe) == 0 {
			return nil, errors.New("stdin is empty and input flag is not specified")
		}
	}

	if _, err := platformutil.NewMatchComparer(options.AllPlatforms, options.Platform); err != nil {
		return nil, err
	}

	imageService := client.ImageService()
	beforeImages, err := imageService.List(ctx)
	if err != nil {
		return nil, err
	}
	beforeSet := make(map[string]bool)
	for _, img := range beforeImages {
		beforeSet[img.Name] = true
	}

	var storeOpts []transferimage.StoreOpt
	platUnpack := platforms.DefaultSpec()
	if len(options.Platform) > 0 {
		p, err := platforms.Parse(options.Platform[0])
		if err != nil {
			return nil, fmt.Errorf("invalid platform %q: %w", options.Platform[0], err)
		}
		platUnpack = p
		storeOpts = append(storeOpts, transferimage.WithPlatforms(p))
	} else if !options.AllPlatforms {
		storeOpts = append(storeOpts, transferimage.WithPlatforms(platUnpack))
	}
	storeOpts = append(storeOpts, transferimage.WithUnpack(platUnpack, options.GOptions.Snapshotter))
	storeOpts = append(storeOpts, transferimage.WithDigestRef("import", true, true))

	var loadedImages []images.Image
	pf, done := transferutil.ProgressHandler(ctx, options.Stdout)
	defer done()

	err = client.Transfer(ctx,
		tarchive.NewImageImportStream(options.Stdin, ""),
		transferimage.NewStore("", storeOpts...),
		transfer.WithProgress(func(p transfer.Progress) {
			if p.Event == "saved" {
				if img, err := imageService.Get(ctx, p.Name); err == nil {
					if !beforeSet[img.Name] {
						loadedImages = append(loadedImages, img)
						if !options.Quiet {
							fmt.Fprintf(options.Stdout, "Loaded image: %s\n", img.Name)
						}
					}
				}
			}
			pf(p)
		}),
	)

	return loadedImages, err
}

// FromOCIArchive loads and unpacks the images from the OCI formatted archive at the provided file system path.
func FromOCIArchive(ctx context.Context, client *containerd.Client, pathToOCIArchive string, options types.ImageLoadOptions) ([]images.Image, error) {
	const ociArchivePrefix = "oci-archive://"
	pathToOCIArchive = strings.TrimPrefix(pathToOCIArchive, ociArchivePrefix)

	const separator = ":"
	if strings.Contains(pathToOCIArchive, separator) {
		subs := strings.Split(pathToOCIArchive, separator)
		if len(subs) != 2 {
			return nil, errors.New("too many seperators found in oci-archive path")
		}
		pathToOCIArchive = subs[0]
	}

	options.Input = pathToOCIArchive

	return FromArchive(ctx, client, options)
}
