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

package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	overlaybdconvert "github.com/containerd/accelerated-container-image/pkg/convertor"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/images/converter"
	"github.com/containerd/containerd/images/converter/uncompress"
	"github.com/containerd/nerdctl/pkg/clientutil"
	converterutil "github.com/containerd/nerdctl/pkg/imgutil/converter"
	"github.com/containerd/nerdctl/pkg/platformutil"
	"github.com/containerd/nerdctl/pkg/referenceutil"
	nydusconvert "github.com/containerd/nydus-snapshotter/pkg/converter"
	"github.com/containerd/stargz-snapshotter/estargz"
	estargzconvert "github.com/containerd/stargz-snapshotter/nativeconverter/estargz"
	estargzexternaltocconvert "github.com/containerd/stargz-snapshotter/nativeconverter/estargz/externaltoc"
	zstdchunkedconvert "github.com/containerd/stargz-snapshotter/nativeconverter/zstdchunked"
	"github.com/containerd/stargz-snapshotter/recorder"
	"github.com/klauspost/compress/zstd"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const imageConvertHelp = `Convert an image format.

e.g., 'nerdctl image convert --estargz --oci example.com/foo:orig example.com/foo:esgz'

Use '--platform' to define the output platform.
When '--all-platforms' is given all images in a manifest list must be available.

For encryption and decryption, use 'nerdctl image (encrypt|decrypt)' command.
`

// imageConvertCommand is from https://github.com/containerd/stargz-snapshotter/blob/d58f43a8235e46da73fb94a1a35280cb4d607b2c/cmd/ctr-remote/commands/convert.go
func newImageConvertCommand() *cobra.Command {
	imageConvertCommand := &cobra.Command{
		Use:               "convert [flags] <source_ref> <target_ref>...",
		Short:             "convert an image",
		Long:              imageConvertHelp,
		Args:              cobra.MinimumNArgs(2),
		RunE:              imageConvertAction,
		ValidArgsFunction: imageConvertShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}

	imageConvertCommand.Flags().String("format", "", "Format the output using the given Go template, e.g, 'json'")

	// #region estargz flags
	imageConvertCommand.Flags().Bool("estargz", false, "Convert legacy tar(.gz) layers to eStargz for lazy pulling. Should be used in conjunction with '--oci'")
	imageConvertCommand.Flags().String("estargz-record-in", "", "Read 'ctr-remote optimize --record-out=<FILE>' record file (EXPERIMENTAL)")
	imageConvertCommand.Flags().Int("estargz-compression-level", gzip.BestCompression, "eStargz compression level")
	imageConvertCommand.Flags().Int("estargz-chunk-size", 0, "eStargz chunk size")
	imageConvertCommand.Flags().Int("estargz-min-chunk-size", 0, "The minimal number of bytes of data must be written in one gzip stream. (requires stargz-snapshotter >= v0.13.0)")
	imageConvertCommand.Flags().Bool("estargz-external-toc", false, "Separate TOC JSON into another image (called \"TOC image\"). The name of TOC image is the original + \"-esgztoc\" suffix. Both eStargz and the TOC image should be pushed to the same registry. (requires stargz-snapshotter >= v0.13.0) (EXPERIMENTAL)")
	imageConvertCommand.Flags().Bool("estargz-keep-diff-id", false, "Convert to esgz without changing diffID (cannot be used in conjunction with '--estargz-record-in'. must be specified with '--estargz-external-toc')")
	// #endregion

	// #region zstd:chunked flags
	imageConvertCommand.Flags().Bool("zstdchunked", false, "Convert legacy tar(.gz) layers to zstd:chunked for lazy pulling. Should be used in conjunction with '--oci'")
	imageConvertCommand.Flags().String("zstdchunked-record-in", "", "Read 'ctr-remote optimize --record-out=<FILE>' record file (EXPERIMENTAL)")
	imageConvertCommand.Flags().Int("zstdchunked-compression-level", 3, "zstd:chunked compression level") // SpeedDefault; see also https://pkg.go.dev/github.com/klauspost/compress/zstd#EncoderLevel
	imageConvertCommand.Flags().Int("zstdchunked-chunk-size", 0, "zstd:chunked chunk size")
	// #endregion

	// #region nydus flags
	imageConvertCommand.Flags().Bool("nydus", false, "Convert an OCI image to Nydus image. Should be used in conjunction with '--oci'")
	imageConvertCommand.Flags().String("nydus-builder-path", "nydus-image", "The nydus-image binary path, if unset, search in PATH environment")
	imageConvertCommand.Flags().String("nydus-work-dir", "", "Work directory path for image conversion, default is the nerdctl data root directory")
	imageConvertCommand.Flags().String("nydus-prefetch-patterns", "", "The file path pattern list want to prefetch")
	imageConvertCommand.Flags().String("nydus-compressor", "lz4_block", "Nydus blob compression algorithm, possible values: `none`, `lz4_block`, `zstd`, default is `lz4_block`")
	// #endregion

	// #region overlaybd flags
	imageConvertCommand.Flags().Bool("overlaybd", false, "Convert tar.gz layers to overlaybd layers")
	imageConvertCommand.Flags().String("overlaybd-fs-type", "ext4", "Filesystem type for overlaybd")
	imageConvertCommand.Flags().String("overlaybd-dbstr", "", "Database config string for overlaybd")
	// #endregion

	// #region generic flags
	imageConvertCommand.Flags().Bool("uncompress", false, "Convert tar.gz layers to uncompressed tar layers")
	imageConvertCommand.Flags().Bool("oci", false, "Convert Docker media types to OCI media types")
	// #endregion

	// #region platform flags
	// platform is defined as StringSlice, not StringArray, to allow specifying "--platform=amd64,arm64"
	imageConvertCommand.Flags().StringSlice("platform", []string{}, "Convert content for a specific platform")
	imageConvertCommand.RegisterFlagCompletionFunc("platform", shellCompletePlatforms)
	imageConvertCommand.Flags().Bool("all-platforms", false, "Convert content for all platforms")
	// #endregion

	return imageConvertCommand
}

func imageConvertAction(cmd *cobra.Command, args []string) error {
	var (
		convertOpts = []converter.Opt{}
	)
	srcRawRef := args[0]
	targetRawRef := args[1]
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

	allPlatforms, err := cmd.Flags().GetBool("all-platforms")
	if err != nil {
		return err
	}
	platform, err := cmd.Flags().GetStringSlice("platform")
	if err != nil {
		return err
	}
	platMC, err := platformutil.NewMatchComparer(allPlatforms, platform)
	if err != nil {
		return err
	}
	convertOpts = append(convertOpts, converter.WithPlatform(platMC))

	estargz, err := cmd.Flags().GetBool("estargz")
	if err != nil {
		return err
	}
	zstdchunked, err := cmd.Flags().GetBool("zstdchunked")
	if err != nil {
		return err
	}
	nydus, err := cmd.Flags().GetBool("nydus")
	if err != nil {
		return err
	}
	overlaybd, err := cmd.Flags().GetBool("overlaybd")
	if err != nil {
		return err
	}
	oci, err := cmd.Flags().GetBool("oci")
	if err != nil {
		return err
	}
	uncompressValue, err := cmd.Flags().GetBool("uncompress")
	if err != nil {
		return err
	}
	namespace, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	}
	address, err := cmd.Flags().GetString("address")
	if err != nil {
		return err
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), namespace, address)
	if err != nil {
		return err
	}
	defer cancel()

	var finalize func(ctx context.Context, cs content.Store, ref string, desc *ocispec.Descriptor) (*images.Image, error)
	if estargz || zstdchunked || overlaybd || nydus {
		convertCount := 0
		if estargz {
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
			convertFunc, finalize, err = getESGZConverter(cmd)
			if err != nil {
				return err
			}
			convertType = "estargz"
		case zstdchunked:
			convertFunc, err = getZstdchunkedConverter(cmd)
			if err != nil {
				return err
			}
			convertType = "zstdchunked"
		case overlaybd:
			obdOpts, err := getOBDConvertOpts(cmd)
			if err != nil {
				return err
			}
			obdOpts = append(obdOpts, overlaybdconvert.WithClient(client))
			obdOpts = append(obdOpts, overlaybdconvert.WithImageRef(srcRef))
			convertFunc = overlaybdconvert.IndexConvertFunc(obdOpts...)
			convertOpts = append(convertOpts, converter.WithIndexConvertFunc(convertFunc))
			convertType = "overlaybd"
		case nydus:
			nydusOpts, err := getNydusConvertOpts(cmd)
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
		if !oci {
			if nydus || overlaybd {
				logrus.Warnf("option --%s should be used in conjunction with --oci, forcibly enabling on oci mediatype for %s conversion", convertType, convertType)
			} else {
				logrus.Warnf("option --%s should be used in conjunction with --oci", convertType)
			}
		}
		if uncompressValue {
			return fmt.Errorf("option --%s conflicts with --uncompress", convertType)
		}
	}

	if uncompressValue {
		convertOpts = append(convertOpts, converter.WithLayerConvertFunc(uncompress.LayerConvertFunc))
	}

	if oci {
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
	return printConvertedImage(cmd, res)
}

func getESGZConverter(cmd *cobra.Command) (convertFunc converter.ConvertFunc, finalize func(ctx context.Context, cs content.Store, ref string, desc *ocispec.Descriptor) (*images.Image, error), _ error) {
	experimental, err := cmd.Flags().GetBool("experimental")
	if err != nil {
		return nil, nil, err
	}

	externalTOC, err := cmd.Flags().GetBool("estargz-external-toc")
	if err != nil {
		return nil, nil, err
	}
	keepDiffID, err := cmd.Flags().GetBool("estargz-keep-diff-id")
	if err != nil {
		return nil, nil, err
	}
	if externalTOC && !experimental {
		return nil, nil, fmt.Errorf("estargz-external-toc requires experimental mode to be enabled")
	}
	if keepDiffID && !externalTOC {
		return nil, nil, fmt.Errorf("option --estargz-keep-diff-id must be specified with --estargz-external-toc")
	}
	if externalTOC {
		estargzCompressionLevel, err := cmd.Flags().GetInt("estargz-compression-level")
		if err != nil {
			return nil, nil, err
		}
		if !keepDiffID {
			esgzOpts, err := getESGZConvertOpts(cmd)
			if err != nil {
				return nil, nil, err
			}
			convertFunc, finalize = estargzexternaltocconvert.LayerConvertFunc(esgzOpts, estargzCompressionLevel)
		} else {
			estargzChunkSize, err := cmd.Flags().GetInt("estargz-chunk-size")
			if err != nil {
				return nil, nil, err
			}
			estargzMinChunkSize, err := cmd.Flags().GetInt("estargz-min-chunk-size")
			if err != nil {
				return nil, nil, err
			}
			convertFunc, finalize = estargzexternaltocconvert.LayerConvertLossLessFunc(estargzexternaltocconvert.LayerConvertLossLessConfig{
				CompressionLevel: estargzCompressionLevel,
				ChunkSize:        estargzChunkSize,
				MinChunkSize:     estargzMinChunkSize,
			})
		}
	} else {
		esgzOpts, err := getESGZConvertOpts(cmd)
		if err != nil {
			return nil, nil, err
		}
		convertFunc = estargzconvert.LayerConvertFunc(esgzOpts...)
	}
	return convertFunc, finalize, nil
}

func getESGZConvertOpts(cmd *cobra.Command) ([]estargz.Option, error) {
	estargzCompressionLevel, err := cmd.Flags().GetInt("estargz-compression-level")
	if err != nil {
		return nil, err
	}
	estargzChunkSize, err := cmd.Flags().GetInt("estargz-chunk-size")
	if err != nil {
		return nil, err
	}
	estargzMinChunkSize, err := cmd.Flags().GetInt("estargz-min-chunk-size")
	if err != nil {
		return nil, err
	}
	estargzRecordIn, err := cmd.Flags().GetString("estargz-record-in")
	if err != nil {
		return nil, err
	}

	esgzOpts := []estargz.Option{
		estargz.WithCompressionLevel(estargzCompressionLevel),
		estargz.WithChunkSize(estargzChunkSize),
		estargz.WithMinChunkSize(estargzMinChunkSize),
	}

	experimental, err := cmd.Flags().GetBool("experimental")
	if err != nil {
		return nil, err
	}

	if estargzRecordIn != "" {
		if !experimental {
			return nil, fmt.Errorf("estargz-record-in requires experimental mode to be enabled")
		}

		logrus.Warn("--estargz-record-in flag is experimental and subject to change")
		paths, err := readPathsFromRecordFile(estargzRecordIn)
		if err != nil {
			return nil, err
		}
		esgzOpts = append(esgzOpts, estargz.WithPrioritizedFiles(paths))
		var ignored []string
		esgzOpts = append(esgzOpts, estargz.WithAllowPrioritizeNotFound(&ignored))
	}
	return esgzOpts, nil
}

func getZstdchunkedConverter(cmd *cobra.Command) (converter.ConvertFunc, error) {
	zstdchunkedCompressionLevel, err := cmd.Flags().GetInt("zstdchunked-compression-level")
	if err != nil {
		return nil, err
	}
	zstdchunkedChunkSize, err := cmd.Flags().GetInt("zstdchunked-chunk-size")
	if err != nil {
		return nil, err
	}
	zstdchunkedRecordIn, err := cmd.Flags().GetString("zstdchunked-record-in")
	if err != nil {
		return nil, err
	}

	esgzOpts := []estargz.Option{
		estargz.WithChunkSize(zstdchunkedChunkSize),
	}

	experimental, err := cmd.Flags().GetBool("experimental")
	if err != nil {
		return nil, err
	}

	if zstdchunkedRecordIn != "" {
		if !experimental {
			return nil, fmt.Errorf("zstdchunked-record-in requires experimental mode to be enabled")
		}

		logrus.Warn("--zstdchunked-record-in flag is experimental and subject to change")
		paths, err := readPathsFromRecordFile(zstdchunkedRecordIn)
		if err != nil {
			return nil, err
		}
		esgzOpts = append(esgzOpts, estargz.WithPrioritizedFiles(paths))
		var ignored []string
		esgzOpts = append(esgzOpts, estargz.WithAllowPrioritizeNotFound(&ignored))
	}
	return zstdchunkedconvert.LayerConvertFuncWithCompressionLevel(zstd.EncoderLevelFromZstd(zstdchunkedCompressionLevel), esgzOpts...), nil
}

func getNydusConvertOpts(cmd *cobra.Command) (*nydusconvert.PackOption, error) {
	builderPath, err := cmd.Flags().GetString("nydus-builder-path")
	if err != nil {
		return nil, err
	}
	workDir, err := cmd.Flags().GetString("nydus-work-dir")
	if err != nil {
		return nil, err
	}
	dataRoot, err := cmd.Flags().GetString("data-root")
	if err != nil {
		return nil, err
	}
	address, err := cmd.Flags().GetString("address")
	if err != nil {
		return nil, err
	}
	if workDir == "" {
		workDir, err = clientutil.DataStore(dataRoot, address)
		if err != nil {
			return nil, err
		}
	}
	prefetchPatterns, err := cmd.Flags().GetString("nydus-prefetch-patterns")
	if err != nil {
		return nil, err
	}
	compressor, err := cmd.Flags().GetString("nydus-compressor")
	if err != nil {
		return nil, err
	}
	return &nydusconvert.PackOption{
		BuilderPath: builderPath,
		// the path will finally be used is <NERDCTL_DATA_ROOT>/nydus-converter-<hash>,
		// for example: /var/lib/nerdctl/1935db59/nydus-converter-3269662176/,
		// and it will be deleted after the conversion
		WorkDir:          workDir,
		PrefetchPatterns: prefetchPatterns,
		Compressor:       compressor,
		FsVersion:        "6",
	}, nil
}

func getOBDConvertOpts(cmd *cobra.Command) ([]overlaybdconvert.Option, error) {
	obdFsType, err := cmd.Flags().GetString("overlaybd-fs-type")
	if err != nil {
		return nil, err
	}
	obdDbstr, err := cmd.Flags().GetString("overlaybd-dbstr")
	if err != nil {
		return nil, err
	}

	obdOpts := []overlaybdconvert.Option{
		overlaybdconvert.WithFsType(obdFsType),
		overlaybdconvert.WithDbstr(obdDbstr),
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

func imageConvertShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show image names
	return shellCompleteImageNames(cmd)
}

func printConvertedImage(cmd *cobra.Command, img converterutil.ConvertedImageInfo) error {
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	switch format {
	case "json":
		b, err := json.MarshalIndent(img, "", "    ")
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(b))
	default:
		for i, e := range img.ExtraImages {
			elems := strings.SplitN(e, "@", 2)
			if len(elems) < 2 {
				logrus.Errorf("extra reference %q doesn't contain digest", e)
			} else {
				logrus.Infof("Extra image(%d) %s", i, elems[0])
			}
		}
		elems := strings.SplitN(img.Image, "@", 2)
		if len(elems) < 2 {
			logrus.Errorf("reference %q doesn't contain digest", img.Image)
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), elems[1])
		}
	}
	return nil
}
