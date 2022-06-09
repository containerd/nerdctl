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
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/containerd/containerd/images/converter"
	"github.com/containerd/containerd/images/converter/uncompress"
	"github.com/containerd/nerdctl/pkg/platformutil"
	"github.com/containerd/nerdctl/pkg/referenceutil"
	"github.com/containerd/stargz-snapshotter/estargz"
	estargzconvert "github.com/containerd/stargz-snapshotter/nativeconverter/estargz"
	zstdchunkedconvert "github.com/containerd/stargz-snapshotter/nativeconverter/zstdchunked"
	"github.com/containerd/stargz-snapshotter/recorder"

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

	// #region estargz flags
	imageConvertCommand.Flags().Bool("estargz", false, "Convert legacy tar(.gz) layers to eStargz for lazy pulling. Should be used in conjunction with '--oci'")
	imageConvertCommand.Flags().String("estargz-record-in", "", "Read 'ctr-remote optimize --record-out=<FILE>' record file (EXPERIMENTAL)")
	imageConvertCommand.Flags().Int("estargz-compression-level", gzip.BestCompression, "eStargz compression level")
	imageConvertCommand.Flags().Int("estargz-chunk-size", 0, "eStargz chunk size")
	imageConvertCommand.Flags().Bool("zstdchunked", false, "Use zstd compression instead of gzip (a.k.a zstd:chunked). Should be used in conjunction with '--oci'")
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
	oci, err := cmd.Flags().GetBool("oci")
	if err != nil {
		return err
	}
	uncompressValue, err := cmd.Flags().GetBool("uncompress")
	if err != nil {
		return err
	}

	if estargz || zstdchunked {
		if estargz && zstdchunked {
			return errors.New("option --estargz conflicts with --zstdchunked")
		}

		esgzOpts, err := getESGZConvertOpts(cmd)
		if err != nil {
			return err
		}

		var convertFunc converter.ConvertFunc
		var convertType string
		switch {
		case estargz:
			convertFunc = estargzconvert.LayerConvertFunc(esgzOpts...)
			convertType = "estargz"
		case zstdchunked:
			convertFunc = zstdchunkedconvert.LayerConvertFunc(esgzOpts...)
			convertType = "zstdchunked"
		}
		convertOpts = append(convertOpts, converter.WithLayerConvertFunc(convertFunc))

		if !oci {
			logrus.Warnf("option --%s should be used in conjunction with --oci", convertType)
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

	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	// converter.Convert() gains the lease by itself
	newImg, err := converter.Convert(ctx, client, targetRef, srcRef, convertOpts...)
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), newImg.Target.Digest.String())
	return nil
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
	estargzRecordIn, err := cmd.Flags().GetString("estargz-record-in")
	if err != nil {
		return nil, err
	}

	esgzOpts := []estargz.Option{
		estargz.WithCompressionLevel(estargzCompressionLevel),
		estargz.WithChunkSize(estargzChunkSize),
	}
	if estargzRecordIn != "" {
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
