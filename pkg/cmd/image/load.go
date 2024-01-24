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
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/archive/compression"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/images/archive"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/imgutil"
	"github.com/containerd/nerdctl/v2/pkg/platformutil"
	"github.com/containerd/platforms"
)

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

func Load(ctx context.Context, client *containerd.Client, options types.ImageLoadOptions) error {
	if options.Input != "" {
		f, err := os.Open(options.Input)
		if err != nil {
			return err
		}
		defer f.Close()
		options.Stdin = f
	} else {
		// check if stdin is empty.
		stdinStat, err := os.Stdin.Stat()
		if err != nil {
			return err
		}
		if stdinStat.Size() == 0 && (stdinStat.Mode()&os.ModeNamedPipe) == 0 {
			return errors.New("stdin is empty and input flag is not specified")
		}
	}
	decompressor, err := compression.DecompressStream(options.Stdin)
	if err != nil {
		return err
	}
	platMC, err := platformutil.NewMatchComparer(options.AllPlatforms, options.Platform)
	if err != nil {
		return err
	}
	return loadImage(ctx, client, decompressor, platMC, false, options)
}

func loadImage(ctx context.Context, client *containerd.Client, in io.Reader, platMC platforms.MatchComparer, quiet bool, options types.ImageLoadOptions) error {
	// In addition to passing WithImagePlatform() to client.Import(), we also need to pass WithDefaultPlatform() to NewClient().
	// Otherwise unpacking may fail.
	r := &readCounter{Reader: in}
	imgs, err := client.Import(ctx, r, containerd.WithDigestRef(archive.DigestTranslator(options.GOptions.Snapshotter)), containerd.WithSkipDigestRef(func(name string) bool { return name != "" }), containerd.WithImportPlatform(platMC))
	if err != nil {
		if r.N == 0 {
			// Avoid confusing "unrecognized image format"
			return errors.New("no image was built")
		}
		if errors.Is(err, images.ErrEmptyWalk) {
			err = fmt.Errorf("%w (Hint: set `--platform=PLATFORM` or `--all-platforms`)", err)
		}
		return err
	}
	for _, img := range imgs {
		image := containerd.NewImageWithPlatform(client, img, platMC)

		// TODO: Show unpack status
		if !quiet {
			fmt.Fprintf(options.Stdout, "unpacking %s (%s)...\n", img.Name, img.Target.Digest)
		}
		err = image.Unpack(ctx, options.GOptions.Snapshotter)
		if err != nil {
			return err
		}
		if quiet {
			fmt.Fprintln(options.Stdout, img.Target.Digest)
		} else {
			repo, tag := imgutil.ParseRepoTag(img.Name)
			fmt.Fprintf(options.Stdout, "Loaded image: %s:%s\n", repo, tag)
		}
	}

	return nil
}
