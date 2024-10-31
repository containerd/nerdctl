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
	"sync"

	"github.com/klauspost/compress/zstd"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/images/converter"
	"github.com/containerd/containerd/v2/core/images/converter/uncompress"
	"github.com/containerd/containerd/v2/pkg/archive/compression"
	"github.com/containerd/containerd/v2/pkg/labels"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"
	"github.com/containerd/stargz-snapshotter/estargz"
	"github.com/containerd/stargz-snapshotter/estargz/zstdchunked"
	"github.com/containerd/stargz-snapshotter/util/ioutils"
)

type zstdCompression struct {
	*zstdchunked.Decompressor
	*zstdchunked.Compressor
}

type unMap struct {
	mu sync.Mutex
}

func LayerConvertFuncWithCompressionLevel(compressionLevel zstd.EncoderLevel, opts ...estargz.Option) converter.ConvertFunc {
	var mu sync.Mutex

	uncompressMap := map[digest.Digest]*unMap{}

	return func(ctx context.Context, cs content.Store, desc ocispec.Descriptor) (*ocispec.Descriptor, error) {
		if !images.IsLayerType(desc.MediaType) {
			// No conversion. No need to return an error here.
			return nil, nil
		}
		uncompressedDesc := &desc
		// We need to uncompress the archive first
		if !uncompress.IsUncompressedType(desc.MediaType) {
			mu.Lock()
			if _, ok := uncompressMap[desc.Digest]; !ok {
				uncompressMap[desc.Digest] = &unMap{}
			}
			uncompressMap[desc.Digest].mu.Lock()
			mu.Unlock()
			defer uncompressMap[desc.Digest].mu.Unlock()

			var err error
			uncompressedDesc, err = uncompress.LayerConvertFunc(ctx, cs, desc)
			if err != nil {
				return nil, err
			}
			if uncompressedDesc == nil {
				return nil, fmt.Errorf("unexpectedly got the same blob after compression (%s, %q)", desc.Digest, desc.MediaType)
			}
			log.G(ctx).Debugf("zstdchunked: uncompressed %s into %s", desc.Digest, uncompressedDesc.Digest)

			defer func() {
				log.G(ctx).Debugf("zstdchunked: garbage collecting %s", uncompressedDesc.Digest)
				if err := cs.Delete(ctx, uncompressedDesc.Digest); err != nil {
					log.G(ctx).WithError(err).WithField("uncompressedDesc", uncompressedDesc).Warn("failed to remove tmp uncompressed layer")
				}
			}()
		}

		info, err := cs.Info(ctx, desc.Digest)
		if err != nil {
			return nil, err
		}
		labelz := info.Labels
		if labelz == nil {
			labelz = make(map[string]string)
		}

		uncompressedReaderAt, err := cs.ReaderAt(ctx, *uncompressedDesc)
		if err != nil {
			return nil, err
		}
		defer uncompressedReaderAt.Close()
		uncompressedSR := io.NewSectionReader(uncompressedReaderAt, 0, uncompressedDesc.Size)
		metadata := make(map[string]string)
		opts = append(opts, estargz.WithCompression(&zstdCompression{
			new(zstdchunked.Decompressor),
			&zstdchunked.Compressor{
				CompressionLevel: compressionLevel,
				Metadata:         metadata,
			},
		}))
		blob, err := estargz.Build(uncompressedSR, append(opts, estargz.WithContext(ctx))...)
		if err != nil {
			return nil, err
		}
		defer blob.Close()
		ref := fmt.Sprintf("convert-zstdchunked-from-%s", desc.Digest)
		w, err := cs.Writer(ctx, content.WithRef(ref))
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

		// Copy and count the contents
		pr, pw := io.Pipe()
		c := new(ioutils.CountWriter)
		doneCount := make(chan struct{})
		go func() {
			defer close(doneCount)
			defer pr.Close()
			decompressR, err := compression.DecompressStream(pr)
			if err != nil {
				pr.CloseWithError(err)
				return
			}
			defer decompressR.Close()
			if _, err := io.Copy(c, decompressR); err != nil {
				pr.CloseWithError(err)
				return
			}
		}()
		n, err := io.Copy(w, io.TeeReader(blob, pw))
		if err != nil {
			return nil, err
		}
		if err := blob.Close(); err != nil {
			return nil, err
		}
		// update diffID label
		labelz[labels.LabelUncompressed] = blob.DiffID().String()
		if err = w.Commit(ctx, n, "", content.WithLabels(labelz)); err != nil && !errdefs.IsAlreadyExists(err) {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		newDesc := desc
		newDesc.MediaType, err = convertMediaTypeToZstd(newDesc.MediaType)
		if err != nil {
			return nil, err
		}
		newDesc.Digest = w.Digest()
		newDesc.Size = n
		if newDesc.Annotations == nil {
			newDesc.Annotations = make(map[string]string, 1)
		}
		tocDgst := blob.TOCDigest().String()
		newDesc.Annotations[estargz.TOCJSONDigestAnnotation] = tocDgst
		newDesc.Annotations[estargz.StoreUncompressedSizeAnnotation] = fmt.Sprintf("%d", c.Size())
		if p, ok := metadata[zstdchunked.ManifestChecksumAnnotation]; ok {
			newDesc.Annotations[zstdchunked.ManifestChecksumAnnotation] = p
		}
		if p, ok := metadata[zstdchunked.ManifestPositionAnnotation]; ok {
			newDesc.Annotations[zstdchunked.ManifestPositionAnnotation] = p
		}
		return &newDesc, nil
	}
}

// NOTE: this converts docker mediatype to OCI mediatype
func convertMediaTypeToZstd(mt string) (string, error) {
	ociMediaType := converter.ConvertDockerMediaTypeToOCI(mt)
	switch ociMediaType {
	case ocispec.MediaTypeImageLayer, ocispec.MediaTypeImageLayerGzip, ocispec.MediaTypeImageLayerZstd:
		return ocispec.MediaTypeImageLayerZstd, nil
	case ocispec.MediaTypeImageLayerNonDistributable, ocispec.MediaTypeImageLayerNonDistributableGzip, ocispec.MediaTypeImageLayerNonDistributableZstd: //nolint:staticcheck // deprecated
		return ocispec.MediaTypeImageLayerNonDistributableZstd, nil //nolint:staticcheck // deprecated
	default:
		return "", fmt.Errorf("unknown mediatype %q", mt)
	}
}
