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

package converter

import (
	"context"
	"fmt"
	"io"

	"github.com/containerd/containerd/archive/compression"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/images/converter"
	"github.com/containerd/containerd/images/converter/uncompress"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/klauspost/compress/zstd"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// ZstdLayerConvertFunc converts legacy tar.gz layers into zstd layers with
// the specified compression level.
func ZstdLayerConvertFunc(options types.ImageConvertOptions) (converter.ConvertFunc, error) {
	return func(ctx context.Context, cs content.Store, desc ocispec.Descriptor) (*ocispec.Descriptor, error) {
		if !images.IsLayerType(desc.MediaType) {
			// No conversion. No need to return an error here.
			return nil, nil
		}
		var err error
		// Read it
		readerAt, err := cs.ReaderAt(ctx, desc)
		if err != nil {
			return nil, err
		}
		defer readerAt.Close()
		sectionReader := io.NewSectionReader(readerAt, 0, desc.Size)

		info, err := cs.Info(ctx, desc.Digest)
		if err != nil {
			return nil, err
		}

		var oldReader io.Reader
		// If it is compressed, get a decompressed stream
		if !uncompress.IsUncompressedType(desc.MediaType) {
			decompStream, err := compression.DecompressStream(sectionReader)
			if err != nil {
				return nil, err
			}
			defer decompStream.Close()
			oldReader = decompStream
		} else {
			oldReader = sectionReader
		}

		ref := fmt.Sprintf("convert-zstd-from-%s", desc.Digest)
		w, err := content.OpenWriter(ctx, cs, content.WithRef(ref))
		if err != nil {
			return nil, err
		}
		defer w.Close()

		// Reset the writing position
		// Old writer possibly remains without aborted
		// (e.g. conversion interrupted by a signal)
		if err := w.Truncate(0); err != nil {
			return nil, err
		}

		pr, pw := io.Pipe()
		enc, err := zstd.NewWriter(pw, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(options.ZstdCompressionLevel)))
		if err != nil {
			return nil, err
		}
		go func() {
			if _, err := io.Copy(enc, oldReader); err != nil {
				pr.CloseWithError(err)
				return
			}
			if err = enc.Close(); err != nil {
				pr.CloseWithError(err)
				return
			}
			if err = pw.Close(); err != nil {
				pr.CloseWithError(err)
				return
			}
		}()

		n, err := io.Copy(w, pr)
		if err != nil {
			return nil, err
		}

		if err = w.Commit(ctx, 0, "", content.WithLabels(info.Labels)); err != nil && !errdefs.IsAlreadyExists(err) {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		newDesc := desc
		newDesc.Digest = w.Digest()
		newDesc.Size = n
		newDesc.MediaType = ocispec.MediaTypeImageLayerZstd
		return &newDesc, nil
	}, nil
}
