/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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
	"io"
	"os"

	"github.com/containerd/containerd/images/archive"
	"github.com/containerd/containerd/platforms"
	refdocker "github.com/containerd/containerd/reference/docker"
	"github.com/mattn/go-isatty"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

var saveCommand = &cli.Command{
	Name:         "save",
	Usage:        "Save one or more images to a tar archive (streamed to STDOUT by default)",
	Description:  "The archive implements both Docker Image Spec v1.2 and OCI Image Spec v1.0.",
	Action:       saveAction,
	BashComplete: saveBashComplete,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "output",
			Aliases: []string{"o"},
			Usage:   "Write to a file, instead of STDOUT",
		},
	},
}

func saveAction(clicontext *cli.Context) error {
	if clicontext.NArg() == 0 {
		return errors.Errorf("requires at least 1 argument")
	}

	var (
		images   = clicontext.Args().Slice()
		saveOpts = []archive.ExportOpt{}
	)

	if len(images) == 0 {
		return errors.Errorf("requires at least 1 argument")
	}

	out := clicontext.App.Writer
	if output := clicontext.String("output"); output != "" {
		f, err := os.OpenFile(output, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	} else {
		if isatty.IsTerminal(os.Stdout.Fd()) {
			return errors.Errorf("cowardly refusing to save to a terminal. Use the -o flag or redirect")
		}
	}
	return saveImage(images, out, saveOpts, clicontext)
}

func saveImage(images []string, out io.Writer, saveOpts []archive.ExportOpt, clicontext *cli.Context) error {
	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	// Set the default platform
	saveOpts = append(saveOpts, archive.WithPlatform(platforms.DefaultStrict()))

	imageStore := client.ImageService()
	for _, img := range images {
		named, err := refdocker.ParseDockerRef(img)
		if err != nil {
			return err
		}
		saveOpts = append(saveOpts, archive.WithImage(imageStore, named.String()))
	}

	return client.Export(ctx, out, saveOpts...)
}

func saveBashComplete(clicontext *cli.Context) {
	coco := parseCompletionContext(clicontext)
	if coco.boring || coco.flagTakesValue {
		defaultBashComplete(clicontext)
		return
	}
	// show image names
	bashCompleteImageNames(clicontext)
}
