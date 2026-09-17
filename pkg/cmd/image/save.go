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
	"io"
	"os"
	"sync"

	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/transfer"
	tarchive "github.com/containerd/containerd/v2/core/transfer/archive"
	transferimage "github.com/containerd/containerd/v2/core/transfer/image"
	"github.com/containerd/platforms"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/platformutil"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
	"github.com/containerd/nerdctl/v2/pkg/transferutil"
)

// Save exports `images` to a `io.Writer` (e.g., a file writer, or os.Stdout) specified by `options.Stdout`.
func Save(ctx context.Context, client *containerd.Client, images []string, options types.ImageSaveOptions) error {
	images = strutil.DedupeStrSlice(images)

	var exportOpts []tarchive.ExportOpt

	if len(options.Platform) > 0 {
		for _, ps := range options.Platform {
			p, err := platforms.Parse(ps)
			if err != nil {
				return fmt.Errorf("invalid platform %q: %w", ps, err)
			}
			exportOpts = append(exportOpts, tarchive.WithPlatform(p))
		}
	}
	if options.AllPlatforms {
		exportOpts = append(exportOpts, tarchive.WithAllPlatforms)
	}

	platMC, err := platformutil.NewMatchComparer(options.AllPlatforms, options.Platform)
	if err != nil {
		return err
	}

	imageService := client.ImageService()
	var storeOpts []transferimage.StoreOpt
	for _, img := range images {
		var imageRef string

		var dgst digest.Digest
		var err error
		if dgst, err = digest.Parse(img); err != nil {
			if dgst, err = digest.Parse("sha256:" + img); err != nil {
				named, err := reference.ParseNormalizedNamed(img)
				if err != nil {
					return fmt.Errorf("invalid image name %q: %w", img, err)
				}
				imageRef = reference.TagNameOnly(named).String()
				err = EnsureAllContent(ctx, client, imageRef, platMC, options.GOptions)
				if err != nil {
					return err
				}
				storeOpts = append(storeOpts, transferimage.WithExtraReference(imageRef))
				continue
			}
		}

		filters := []string{fmt.Sprintf("target.digest~=^%s$", dgst.String())}
		imageList, err := imageService.List(ctx, filters...)
		if err != nil {
			return fmt.Errorf("failed to list images: %w", err)
		}
		if len(imageList) == 0 {
			return fmt.Errorf("image %q: not found", img)
		}

		imageRef = imageList[0].Name
		err = EnsureAllContent(ctx, client, imageRef, platMC, options.GOptions)
		if err != nil {
			return err
		}
		storeOpts = append(storeOpts, transferimage.WithExtraReference(imageRef))
	}

	w := &signalWriteCloser{Writer: options.Stdout, closed: make(chan struct{})}

	progressOutput := io.Writer(os.Stderr)
	if options.Quiet {
		progressOutput = io.Discard
	}
	pf, done := transferutil.ProgressHandler(ctx, progressOutput)
	defer done()

	if err = client.Transfer(ctx,
		transferimage.NewStore("", storeOpts...),
		tarchive.NewImageExportStream(w, "", exportOpts...),
		transfer.WithProgress(pf),
	); err != nil {
		return err
	}

	// The transfer service hands the archive back over a stream that a goroutine of its own
	// copies into `w`, and Transfer returns as soon as the daemon is done - which is before that
	// goroutine has necessarily drained what is still in flight. The goroutine closes the writer
	// when it is over, so that is what we wait for: returning any earlier hands the caller a
	// truncated archive.
	select {
	case <-w.closed:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// signalWriteCloser is an io.WriteCloser that reports, through the `closed` channel, that it has
// been closed - and tolerates being closed more than once, as io.Closer does not promise not to.
type signalWriteCloser struct {
	io.Writer

	once   sync.Once
	closed chan struct{}
}

func (w *signalWriteCloser) Close() error {
	w.once.Do(func() {
		close(w.closed)
	})

	return nil
}
