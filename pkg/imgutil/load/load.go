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
	"io"
	"os"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/images/archive"
	"github.com/containerd/containerd/v2/pkg/archive/compression"
	"github.com/containerd/platforms"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/imgutil"
	"github.com/containerd/nerdctl/v2/pkg/platformutil"
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
	decompressor, err := compression.DecompressStream(options.Stdin)
	if err != nil {
		return nil, err
	}
	platMC, err := platformutil.NewMatchComparer(options.AllPlatforms, options.Platform)
	if err != nil {
		return nil, err
	}
	imgs, err := importImages(ctx, client, decompressor, options.GOptions.Snapshotter, platMC)
	if err != nil {
		return nil, err
	}
	unpackedImages := make([]images.Image, 0, len(imgs))
	for _, img := range imgs {
		err := unpackImage(ctx, client, img, platMC, options)
		if err != nil {
			return unpackedImages, fmt.Errorf("error unpacking image (%s): %w", img.Name, err)
		}
		unpackedImages = append(unpackedImages, img)
	}
	return unpackedImages, nil
}

type readCounter struct {
	io.Reader
	N int
}

func (r *readCounter) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if n > 0 {
		r.N += n
	}
	return n, err
}

func importImages(ctx context.Context, client *containerd.Client, in io.Reader, snapshotter string, platformMC platforms.MatchComparer) ([]images.Image, error) {
	// In addition to passing WithImagePlatform() to client.Import(), we also need to pass WithDefaultPlatform() to NewClient().
	// Otherwise unpacking may fail.
	r := &readCounter{Reader: in}
	imgs, err := client.Import(ctx, r,
		containerd.WithDigestRef(archive.DigestTranslator(snapshotter)),
		containerd.WithSkipDigestRef(func(name string) bool { return name != "" }),
		containerd.WithImportPlatform(platformMC),
	)
	if err != nil {
		if r.N == 0 {
			// Avoid confusing "unrecognized image format"
			return nil, errors.New("no image was built")
		}
		if errors.Is(err, images.ErrEmptyWalk) {
			err = fmt.Errorf("%w (Hint: set `--platform=PLATFORM` or `--all-platforms`)", err)
		}
		return nil, err
	}
	return imgs, nil
}

func unpackImage(ctx context.Context, client *containerd.Client, model images.Image, platform platforms.MatchComparer, options types.ImageLoadOptions) error {
	image := containerd.NewImageWithPlatform(client, model, platform)

	if !options.Quiet {
		fmt.Fprintf(options.Stdout, "unpacking %s (%s)...\n", model.Name, model.Target.Digest)
	}

	err := image.Unpack(ctx, options.GOptions.Snapshotter)
	if err != nil {
		return err
	}

	// Loaded message is shown even when quiet.
	repo, tag := imgutil.ParseRepoTag(model.Name)
	fmt.Fprintf(options.Stdout, "Loaded image: %s:%s\n", repo, tag)

	return nil
}
