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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/klauspost/compress/zstd"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	overlaybdconvert "github.com/containerd/accelerated-container-image/pkg/convertor"
	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/images/converter"
	"github.com/containerd/containerd/v2/core/images/converter/uncompress"
	"github.com/containerd/log"
	nydusconvert "github.com/containerd/nydus-snapshotter/pkg/converter"
	"github.com/containerd/stargz-snapshotter/estargz"
	estargzconvert "github.com/containerd/stargz-snapshotter/nativeconverter/estargz"
	estargzexternaltocconvert "github.com/containerd/stargz-snapshotter/nativeconverter/estargz/externaltoc"
	zstdchunkedconvert "github.com/containerd/stargz-snapshotter/nativeconverter/zstdchunked"
	"github.com/containerd/stargz-snapshotter/recorder"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	converterutil "github.com/containerd/nerdctl/v2/pkg/imgutil/converter"
	"github.com/containerd/nerdctl/v2/pkg/platformutil"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
)

func Convert(ctx context.Context, client *containerd.Client, srcRawRef, targetRawRef string, options types.ImageConvertOptions) error {
	var (
		convertOpts = []converter.Opt{}
	)
	if srcRawRef == "" || targetRawRef == "" {
		return errors.New("src and target image need to be specified")
	}

	srcNamed, err := referenceutil.ParseAny(srcRawRef)
	if err != nil {
		return err
	}
	srcRef := srcNamed.String()

	targetNamed, err := referenceutil.ParseDockerRef(targetRawRef)
	if err != nil {
		return err
	}
	targetRef := targetNamed.String()

	platMC, err := platformutil.NewMatchComparer(options.AllPlatforms, options.Platforms)
	if err != nil {
		return err
	}
	convertOpts = append(convertOpts, converter.WithPlatform(platMC))

	// Ensure all the layers are here: https://github.com/containerd/nerdctl/issues/3425
	err = EnsureAllContent(ctx, client, srcRawRef, options.GOptions)
	if err != nil {
		return err
	}

	estargz := options.Estargz
	zstd := options.Zstd
	zstdchunked := options.ZstdChunked
	overlaybd := options.Overlaybd
	nydus := options.Nydus
	var finalize func(ctx context.Context, cs content.Store, ref string, desc *ocispec.Descriptor) (*images.Image, error)
	if estargz || zstd || zstdchunked || overlaybd || nydus {
		convertCount := 0
		if estargz {
			convertCount++
		}
		if zstd {
			convertCount++
		}
		if zstdchunked {
			convertCount++
		}
		if overlaybd {
			convertCount++
		}
		if nydus {
			convertCount++
		}

		if convertCount > 1 {
			return errors.New("options --estargz, --zstdchunked, --overlaybd and --nydus lead to conflict, only one of them can be used")
		}

		var convertFunc converter.ConvertFunc
		var convertType string
		switch {
		case estargz:
			convertFunc, finalize, err = getESGZConverter(options)
			if err != nil {
				return err
			}
			convertType = "estargz"
		case zstd:
			convertFunc, err = getZstdConverter(options)
			if err != nil {
				return err
			}
			convertType = "zstd"
		case zstdchunked:
			convertFunc, err = getZstdchunkedConverter(options)
			if err != nil {
				return err
			}
			convertType = "zstdchunked"
		case overlaybd:
			obdOpts, err := getOBDConvertOpts(options)
			if err != nil {
				return err
			}
			obdOpts = append(obdOpts, overlaybdconvert.WithClient(client))
			obdOpts = append(obdOpts, overlaybdconvert.WithImageRef(srcRef))
			convertFunc = overlaybdconvert.IndexConvertFunc(obdOpts...)
			convertOpts = append(convertOpts, converter.WithIndexConvertFunc(convertFunc))
			convertType = "overlaybd"
		case nydus:
			nydusOpts, err := getNydusConvertOpts(options)
			if err != nil {
				return err
			}
			convertHooks := converter.ConvertHooks{
				PostConvertHook: nydusconvert.ConvertHookFunc(nydusconvert.MergeOption{
					WorkDir:          nydusOpts.WorkDir,
					BuilderPath:      nydusOpts.BuilderPath,
					FsVersion:        nydusOpts.FsVersion,
					ChunkDictPath:    nydusOpts.ChunkDictPath,
					PrefetchPatterns: nydusOpts.PrefetchPatterns,
					OCI:              true,
				}),
			}
			convertOpts = append(convertOpts, converter.WithIndexConvertFunc(
				converter.IndexConvertFuncWithHook(
					nydusconvert.LayerConvertFunc(*nydusOpts),
					true,
					platMC,
					convertHooks,
				)),
			)
			convertType = "nydus"
		}

		if convertType != "overlaybd" {
			convertOpts = append(convertOpts, converter.WithLayerConvertFunc(convertFunc))
		}
		if !options.Oci {
			if nydus || overlaybd {
				log.G(ctx).Warnf("option --%s should be used in conjunction with --oci, forcibly enabling on oci mediatype for %s conversion", convertType, convertType)
			} else {
				log.G(ctx).Warnf("option --%s should be used in conjunction with --oci", convertType)
			}
		}
		if options.Uncompress {
			return fmt.Errorf("option --%s conflicts with --uncompress", convertType)
		}
	}

	if options.Uncompress {
		convertOpts = append(convertOpts, converter.WithLayerConvertFunc(uncompress.LayerConvertFunc))
	}

	if options.Oci {
		convertOpts = append(convertOpts, converter.WithDockerToOCI(true))
	}

	// converter.Convert() gains the lease by itself
	newImg, err := converter.Convert(ctx, client, targetRef, srcRef, convertOpts...)
	if err != nil {
		return err
	}
	res := converterutil.ConvertedImageInfo{
		Image: newImg.Name + "@" + newImg.Target.Digest.String(),
	}
	if finalize != nil {
		ctx, done, err := client.WithLease(ctx)
		if err != nil {
			return err
		}
		defer done(ctx)
		newI, err := finalize(ctx, client.ContentStore(), targetRef, &newImg.Target)
		if err != nil {
			return err
		}
		is := client.ImageService()
		_ = is.Delete(ctx, newI.Name)
		finimg, err := is.Create(ctx, *newI)
		if err != nil {
			return err
		}
		res.ExtraImages = append(res.ExtraImages, finimg.Name+"@"+finimg.Target.Digest.String())
	}
	return printConvertedImage(options.Stdout, options, res)
}

func getESGZConverter(options types.ImageConvertOptions) (convertFunc converter.ConvertFunc, finalize func(ctx context.Context, cs content.Store, ref string, desc *ocispec.Descriptor) (*images.Image, error), _ error) {
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
			convertFunc, finalize = estargzexternaltocconvert.LayerConvertFunc(esgzOpts, options.EstargzCompressionLevel)
		} else {
			convertFunc, finalize = estargzexternaltocconvert.LayerConvertLossLessFunc(estargzexternaltocconvert.LayerConvertLossLessConfig{
				CompressionLevel: options.EstargzCompressionLevel,
				ChunkSize:        options.EstargzChunkSize,
				MinChunkSize:     options.EstargzMinChunkSize,
			})
		}
	} else {
		esgzOpts, err := getESGZConvertOpts(options)
		if err != nil {
			return nil, nil, err
		}
		convertFunc = estargzconvert.LayerConvertFunc(esgzOpts...)
	}
	return convertFunc, finalize, nil
}

func getESGZConvertOpts(options types.ImageConvertOptions) ([]estargz.Option, error) {

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
		var ignored []string
		esgzOpts = append(esgzOpts, estargz.WithAllowPrioritizeNotFound(&ignored))
	}
	return esgzOpts, nil
}

func getZstdConverter(options types.ImageConvertOptions) (converter.ConvertFunc, error) {
	return converterutil.ZstdLayerConvertFunc(options)
}

func getZstdchunkedConverter(options types.ImageConvertOptions) (converter.ConvertFunc, error) {

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
		var ignored []string
		esgzOpts = append(esgzOpts, estargz.WithAllowPrioritizeNotFound(&ignored))
	}
	return zstdchunkedconvert.LayerConvertFuncWithCompressionLevel(zstd.EncoderLevelFromZstd(options.ZstdChunkedCompressionLevel), esgzOpts...), nil
}

func getNydusConvertOpts(options types.ImageConvertOptions) (*nydusconvert.PackOption, error) {
	workDir := options.NydusWorkDir
	if workDir == "" {
		var err error
		workDir, err = clientutil.DataStore(options.GOptions.DataRoot, options.GOptions.Address)
		if err != nil {
			return nil, err
		}
	}
	return &nydusconvert.PackOption{
		BuilderPath: options.NydusBuilderPath,
		// the path will finally be used is <NERDCTL_DATA_ROOT>/nydus-converter-<hash>,
		// for example: /var/lib/nerdctl/1935db59/nydus-converter-3269662176/,
		// and it will be deleted after the conversion
		WorkDir:          workDir,
		PrefetchPatterns: options.NydusPrefetchPatterns,
		Compressor:       options.NydusCompressor,
		FsVersion:        "6",
	}, nil
}

func getOBDConvertOpts(options types.ImageConvertOptions) ([]overlaybdconvert.Option, error) {
	obdOpts := []overlaybdconvert.Option{
		overlaybdconvert.WithFsType(options.OverlayFsType),
		overlaybdconvert.WithDbstr(options.OverlaydbDBStr),
	}
	return obdOpts, nil
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

func printConvertedImage(stdout io.Writer, options types.ImageConvertOptions, img converterutil.ConvertedImageInfo) error {
	switch options.Format {
	case "json":
		b, err := json.MarshalIndent(img, "", "    ")
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, string(b))
	default:
		for i, e := range img.ExtraImages {
			elems := strings.SplitN(e, "@", 2)
			if len(elems) < 2 {
				log.L.Errorf("extra reference %q doesn't contain digest", e)
			} else {
				log.L.Infof("Extra image(%d) %s", i, elems[0])
			}
		}
		elems := strings.SplitN(img.Image, "@", 2)
		if len(elems) < 2 {
			log.L.Errorf("reference %q doesn't contain digest", img.Image)
		} else {
			fmt.Fprintln(stdout, elems[1])
		}
	}
	return nil
}
