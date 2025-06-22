//go:build !no_esgz

package converter

import (
	"context"
	"encoding/json"
	"os"

	"github.com/klauspost/compress/zstd"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/images/converter"
	"github.com/containerd/log"
	"github.com/containerd/stargz-snapshotter/estargz"
	estargzconvert "github.com/containerd/stargz-snapshotter/nativeconverter/estargz"
	externaltocconvert "github.com/containerd/stargz-snapshotter/nativeconverter/estargz/externaltoc"
	zstdchunkedconvert "github.com/containerd/stargz-snapshotter/nativeconverter/zstdchunked"
	"github.com/containerd/stargz-snapshotter/recorder"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
)

func ESGZZstdChunkedConvertOpt(options types.ZstdChunkedOptions, experimental bool) (converter.Opt, error) {
	esgzOpts := []estargz.Option{
		estargz.WithChunkSize(options.ZstdChunkedChunkSize),
	}

	if options.ZstdChunkedRecordIn != "" {
		if !experimental {
			return nil, ErrZstdInRequiresExperimental
		}

		log.L.Warn("--zstdchunked-record-in flag is experimental and subject to change")
		paths, err := readPathsFromRecordFile(options.ZstdChunkedRecordIn)
		if err != nil {
			return nil, err
		}
		esgzOpts = append(esgzOpts, estargz.WithPrioritizedFiles(paths))
		var ignored []string
		esgzOpts = append(esgzOpts, estargz.WithAllowPrioritizeNotFound(&ignored))
	}
	convertFunc := zstdchunkedconvert.LayerConvertFuncWithCompressionLevel(zstd.EncoderLevelFromZstd(options.ZstdChunkedCompressionLevel), esgzOpts...)
	return converter.WithLayerConvertFunc(convertFunc), nil
}

func ESGZConvertOpt(options types.EstargzOptions, experimental bool) (convertOpt converter.Opt, finalize func(ctx context.Context, cs content.Store, ref string, desc *ocispec.Descriptor) (*images.Image, error), _ error) {
	if options.EstargzExternalToc && !experimental {
		return nil, nil, ErrESGZTocRequiresExperimental
	}
	if options.EstargzKeepDiffID && !experimental {
		return nil, nil, ErrESGZDiffRequiresExperimental
	}
	var convertFunc converter.ConvertFunc
	if options.EstargzExternalToc {
		if !options.EstargzKeepDiffID {
			esgzOpts, err := getESGZConvertOpts(options, experimental)
			if err != nil {
				return nil, nil, err
			}
			convertFunc, finalize = externaltocconvert.LayerConvertFunc(esgzOpts, options.EstargzCompressionLevel)
		} else {
			convertFunc, finalize = externaltocconvert.LayerConvertLossLessFunc(externaltocconvert.LayerConvertLossLessConfig{
				CompressionLevel: options.EstargzCompressionLevel,
				ChunkSize:        options.EstargzChunkSize,
				MinChunkSize:     options.EstargzMinChunkSize,
			})
		}
	} else {
		esgzOpts, err := getESGZConvertOpts(options, experimental)
		if err != nil {
			return nil, nil, err
		}
		convertFunc = estargzconvert.LayerConvertFunc(esgzOpts...)
	}
	return converter.WithLayerConvertFunc(convertFunc), finalize, nil
}

func getESGZConvertOpts(options types.EstargzOptions, experimental bool) ([]estargz.Option, error) {
	esgzOpts := []estargz.Option{
		estargz.WithCompressionLevel(options.EstargzCompressionLevel),
		estargz.WithChunkSize(options.EstargzChunkSize),
		estargz.WithMinChunkSize(options.EstargzMinChunkSize),
	}

	if options.EstargzRecordIn != "" {
		if !experimental {
			return nil, ErrESGSInRequiresExperimental
		}

		log.L.Warn("--estargz-record-in flag is experimental and subject to change")
		paths, err := readPathsFromRecordFile(options.EstargzRecordIn)
		if err != nil {
			return nil, err
		}
		esgzOpts = append(esgzOpts, estargz.WithPrioritizedFiles(paths))
		var ignored []string
		esgzOpts = append(esgzOpts, estargz.WithAllowPrioritizeNotFound(&ignored))
	}
	return esgzOpts, nil
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
