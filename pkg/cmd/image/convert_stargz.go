//go:build !no_stargz

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
	"os"
	"encoding/json"

	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/images/converter"
	"github.com/containerd/log"
	"github.com/klauspost/compress/zstd"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/containerd/stargz-snapshotter/estargz"
	estargzconvert "github.com/containerd/stargz-snapshotter/nativeconverter/estargz"
	estargzexternaltocconvert "github.com/containerd/stargz-snapshotter/nativeconverter/estargz/externaltoc"
	zstdchunkedconvert "github.com/containerd/stargz-snapshotter/nativeconverter/zstdchunked"
	"github.com/containerd/stargz-snapshotter/recorder"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
)

func getEstargzConvertFunc(options types.ImageConvertOptions) (converter.ConvertFunc, func(ctx context.Context, cs content.Store, ref string, desc *ocispec.Descriptor) (*images.Image, error), error) {
	if options.EstargzExternalToc && !options.GOptions.Experimental {
		return nil, nil, fmt.Errorf("estargz-external-toc requires experimental mode to be enabled")
	}
	if options.EstargzKeepDiffID && !options.GOptions.Experimental {
		return nil, nil, fmt.Errorf("option --estargz-keep-diff-id must be specified with --estargz-external-toc")
	}
	if options.EstargzExternalToc {
		if !options.EstargzKeepDiffID {
			esgzOpts, err := getESGZConvertOpts(options)
			if err != nil {
				return nil, nil, err
			}
			convertFunc, finalize := estargzexternaltocconvert.LayerConvertFunc(esgzOpts, options.EstargzCompressionLevel)
			return convertFunc, finalize, nil
		} else {
			convertFunc, finalize := estargzexternaltocconvert.LayerConvertLossLessFunc(estargzexternaltocconvert.LayerConvertLossLessConfig{
				CompressionLevel: options.EstargzCompressionLevel,
				ChunkSize:        options.EstargzChunkSize,
				MinChunkSize:     options.EstargzMinChunkSize,
			})
			return convertFunc, finalize, nil
		}
	} else {
		esgzOpts, err := getESGZConvertOpts(options)
		if err != nil {
			return nil, nil, err
		}
		convertFunc := estargzconvert.LayerConvertFunc(esgzOpts...)
		return convertFunc, nil, nil
	}
}

func getESGZConvertOpts(options types.ImageConvertOptions) ([]estargz.Option, error) {
	var ignored []string
	esgzOpts := []estargz.Option{
		estargz.WithCompressionLevel(options.EstargzCompressionLevel),
		estargz.WithChunkSize(options.EstargzChunkSize),
		estargz.WithMinChunkSize(options.EstargzMinChunkSize),
	}

	if options.EstargzRecordIn != "" {
		if !options.GOptions.Experimental {
			return nil, fmt.Errorf("estargz-record-in requires experimental mode to be enabled")
		}

		log.L.Warn("--estargz-record-in flag is experimental and subject to change")
		paths, err := readPathsFromRecordFile(options.EstargzRecordIn)
		if err != nil {
			return nil, err
		}
		esgzOpts = append(esgzOpts, estargz.WithPrioritizedFiles(paths))
		// Use default ignored value
		esgzOpts = append(esgzOpts, estargz.WithAllowPrioritizeNotFound(&ignored))
	}

	return esgzOpts, nil
}

func getZstdChunkedConvertFunc(options types.ImageConvertOptions) (converter.ConvertFunc, error) {
	var ignored []string
	esgzOpts := []estargz.Option{
		estargz.WithChunkSize(options.ZstdChunkedChunkSize),
	}

	if options.ZstdChunkedRecordIn != "" {
		if !options.GOptions.Experimental {
			return nil, fmt.Errorf("zstdchunked-record-in requires experimental mode to be enabled")
		}

		log.L.Warn("--zstdchunked-record-in flag is experimental and subject to change")
		paths, err := readPathsFromRecordFile(options.ZstdChunkedRecordIn)
		if err != nil {
			return nil, err
		}
		esgzOpts = append(esgzOpts, estargz.WithPrioritizedFiles(paths))
		// Use default ignored value  
		esgzOpts = append(esgzOpts, estargz.WithAllowPrioritizeNotFound(&ignored))
	}

	convertFunc := zstdchunkedconvert.LayerConvertFuncWithCompressionLevel(zstd.EncoderLevelFromZstd(options.ZstdChunkedCompressionLevel), esgzOpts...)
	return convertFunc, nil
}

func readPathsFromRecordFile(filename string) ([]string, error) {
	r, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	dec := json.NewDecoder(r)
	var paths []string
	added := make(map[string]struct{})
	for dec.More() {
		var e recorder.Entry
		if err := dec.Decode(&e); err != nil {
			return nil, err
		}
		if _, ok := added[e.Path]; !ok {
			paths = append(paths, e.Path)
			added[e.Path] = struct{}{}
		}
	}
	return paths, nil
}