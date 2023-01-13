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
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/platformutil"
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

func Load(ctx context.Context, stdin io.Reader, stdout io.Writer, options types.LoadCommandOptions) error {
	if options.Input != "" {
		f, err := os.Open(options.Input)
		if err != nil {
			return err
		}
		defer f.Close()
		stdin = f
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
	decompressor, err := compression.DecompressStream(stdin)
	if err != nil {
		return err
	}
	platMC, err := platformutil.NewMatchComparer(options.AllPlatforms, options.Platform)
	if err != nil {
		return err
	}
	return loadImage(ctx, decompressor, stdout, options, platMC, false)
}

func loadImage(ctx context.Context, in io.Reader, stdout io.Writer, options types.LoadCommandOptions, platMC platforms.MatchComparer, quiet bool) error {
	// In addition to passing WithImagePlatform() to client.Import(), we also need to pass WithDefaultPlatform() to NewClient().
	// Otherwise unpacking may fail.

	client, ctx, cancel, err := clientutil.NewClient(ctx, options.GOptions.Namespace, options.GOptions.Address, containerd.WithDefaultPlatform(platMC))
	if err != nil {
		return err
	}
	defer cancel()

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
			fmt.Fprintf(stdout, "unpacking %s (%s)...\n", img.Name, img.Target.Digest)
		}
		err = image.Unpack(ctx, options.GOptions.Snapshotter)
		if err != nil {
			return err
		}
		if quiet {
			fmt.Fprintln(stdout, img.Target.Digest)
		} else {
			fmt.Fprintf(stdout, "Loaded image: %s\n", img.Name)
		}
	}

	return nil
}
