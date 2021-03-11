/*
   Copyright The nerdctl Authors.
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
	"github.com/containerd/stargz-snapshotter/estargz"
	estargzconvert "github.com/containerd/stargz-snapshotter/nativeconverter/estargz"
	"github.com/containerd/stargz-snapshotter/recorder"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

// imageConvertCommand is from https://github.com/containerd/stargz-snapshotter/blob/d58f43a8235e46da73fb94a1a35280cb4d607b2c/cmd/ctr-remote/commands/convert.go
var imageConvertCommand = &cli.Command{
	Name:      "convert",
	Usage:     "convert an image",
	ArgsUsage: "[flags] <source_ref> <target_ref>...",
	Description: `Convert an image format.

e.g., 'nerdctl image convert --estargz --oci example.com/foo:orig example.com/foo:esgz'

Use '--platform' to define the output platform.
When '--all-platforms' is given all images in a manifest list must be available.
`,
	Flags: []cli.Flag{
		// estargz flags
		&cli.BoolFlag{
			Name:  "estargz",
			Usage: "Convert legacy tar(.gz) layers to eStargz for lazy pulling. Should be used in conjunction with '--oci'",
		},
		&cli.StringFlag{
			Name:  "estargz-record-in",
			Usage: "Read 'ctr-remote optimize --record-out=<FILE>' record file (EXPERIMENTAL)",
		},
		&cli.IntFlag{
			Name:  "estargz-compression-level",
			Usage: "eStargz compression level",
			Value: gzip.BestCompression,
		},
		&cli.IntFlag{
			Name:  "estargz-chunk-size",
			Usage: "eStargz chunk size",
			Value: 0,
		},
		// generic flags
		&cli.BoolFlag{
			Name:  "uncompress",
			Usage: "Convert tar.gz layers to uncompressed tar layers",
		},
		&cli.BoolFlag{
			Name:  "oci",
			Usage: "Convert Docker media types to OCI media types",
		},
		// platform flags
		&cli.StringSliceFlag{
			Name:  "platform",
			Usage: "Convert content for a specific platform",
			Value: &cli.StringSlice{},
		},
		&cli.BoolFlag{
			Name:  "all-platforms",
			Usage: "Convert content for all platforms",
		},
	},
	Action: func(context *cli.Context) error {
		var (
			convertOpts = []converter.Opt{}
		)
		srcRawRef := context.Args().Get(0)
		targetRawRef := context.Args().Get(1)
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

		if !context.Bool("all-platforms") {
			if pss := context.StringSlice("platform"); len(pss) > 0 {
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

		if context.Bool("estargz") {
			esgzOpts, err := getESGZConvertOpts(context)
			if err != nil {
				return err
			}
			convertOpts = append(convertOpts, converter.WithLayerConvertFunc(estargzconvert.LayerConvertFunc(esgzOpts...)))
			if !context.Bool("oci") {
				logrus.Warn("option --estargz should be used in conjunction with --oci")
			}
			if context.Bool("uncompress") {
				return errors.New("option --estargz conflicts with --uncompress")
			}
		}

		if context.Bool("uncompress") {
			convertOpts = append(convertOpts, converter.WithLayerConvertFunc(uncompress.LayerConvertFunc))
		}

		if context.Bool("oci") {
			convertOpts = append(convertOpts, converter.WithDockerToOCI(true))
		}

		client, ctx, cancel, err := newClient(context)
		if err != nil {
			return err
		}
		defer cancel()

		// converter.Convert() gains the lease by itself
		newImg, err := converter.Convert(ctx, client, targetRef, srcRef, convertOpts...)
		if err != nil {
			return err
		}
		fmt.Fprintln(context.App.Writer, newImg.Target.Digest.String())
		return nil
	},
	BashComplete: imageConvertBashComplete,
}

func getESGZConvertOpts(context *cli.Context) ([]estargz.Option, error) {
	esgzOpts := []estargz.Option{
		estargz.WithCompressionLevel(context.Int("estargz-compression-level")),
		estargz.WithChunkSize(context.Int("estargz-chunk-size")),
	}
	if estargzRecordIn := context.String("estargz-record-in"); estargzRecordIn != "" {
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

func imageConvertBashComplete(clicontext *cli.Context) {
	coco := parseCompletionContext(clicontext)
	if coco.boring || coco.flagTakesValue {
		defaultBashComplete(clicontext)
		return
	}
	// show image names
	bashCompleteImageNames(clicontext)
}
