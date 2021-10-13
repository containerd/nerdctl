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
	"fmt"
	"os"

	"github.com/containerd/containerd/images/converter"
	"github.com/containerd/containerd/images/converter/uncompress"
	"github.com/containerd/containerd/platforms"
	refdocker "github.com/containerd/containerd/reference/docker"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/containerd/stargz-snapshotter/estargz"
	estargzconvert "github.com/containerd/stargz-snapshotter/nativeconverter/estargz"
	"github.com/containerd/stargz-snapshotter/recorder"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const imageConvertHelp = `Convert an image format.

e.g., 'nerdctl image convert --estargz --oci example.com/foo:orig example.com/foo:esgz'

Use '--platform' to define the output platform.
When '--all-platforms' is given all images in a manifest list must be available.
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
	// #endregion

	// #region generic flags
	imageConvertCommand.Flags().Bool("uncompress", false, "Convert tar.gz layers to uncompressed tar layers")
	imageConvertCommand.Flags().Bool("oci", false, "Convert Docker media types to OCI media types")
	// #endregion

	// #region platform flags
	imageConvertCommand.Flags().StringSlice("platform", []string{}, "Convert content for a specific platform")
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

	srcNamed, err := refdocker.ParseDockerRef(srcRawRef)
	if err != nil {
		return err
	}
	srcRef := srcNamed.String()

	targetNamed, err := refdocker.ParseDockerRef(targetRawRef)
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

	if !allPlatforms {
		if pss := strutil.DedupeStrSlice(platform); len(pss) > 0 {
			var all []ocispec.Platform
			for _, ps := range pss {
				p, err := platforms.Parse(ps)
				if err != nil {
					return errors.Wrapf(err, "invalid platform %q", ps)
				}
				all = append(all, p)
			}
			convertOpts = append(convertOpts, converter.WithPlatform(platforms.Ordered(all...)))
		} else {
			convertOpts = append(convertOpts, converter.WithPlatform(platforms.DefaultStrict()))
		}
	}

	estargz, err := cmd.Flags().GetBool("estargz")
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

	if estargz {
		esgzOpts, err := getESGZConvertOpts(cmd)
		if err != nil {
			return err
		}
		convertOpts = append(convertOpts, converter.WithLayerConvertFunc(estargzconvert.LayerConvertFunc(esgzOpts...)))
		if !oci {
			logrus.Warn("option --estargz should be used in conjunction with --oci")
		}
		if uncompressValue {
			return errors.New("option --estargz conflicts with --uncompress")
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
