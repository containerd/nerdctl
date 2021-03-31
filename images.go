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
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/pkg/progress"
	"github.com/containerd/containerd/platforms"
	refdocker "github.com/containerd/containerd/reference/docker"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var imagesCommand = &cli.Command{
	Name:   "images",
	Usage:  "List images",
	Action: imagesAction,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "quiet",
			Aliases: []string{"q"},
			Usage:   "Only show numeric IDs",
		},
		&cli.BoolFlag{
			Name:  "no-trunc",
			Usage: "Don't truncate output",
		},
	},
}

func imagesAction(clicontext *cli.Context) error {
	var filters []string

	if clicontext.NArg() > 1 {
		return errors.New("cannot have more than one argument")
	}

	if clicontext.NArg() > 0 {
		canonicalRef, err := refdocker.ParseDockerRef(clicontext.Args().First())
		if err != nil {
			return err
		}
		filters = append(filters, fmt.Sprintf("name==%s", canonicalRef.String()))
	}
	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	var (
		imageStore = client.ImageService()
		cs         = client.ContentStore()
	)

	// To-do: Add support for --filter.
	imageList, err := imageStore.List(ctx, filters...)
	if err != nil {
		return err
	}

	return printImages(ctx, clicontext, imageList, cs)
}

func printImages(ctx context.Context, clicontext *cli.Context, imageList []images.Image, cs content.Store) error {
	quiet := clicontext.Bool("quiet")
	noTrunc := clicontext.Bool("no-trunc")

	w := tabwriter.NewWriter(clicontext.App.Writer, 4, 8, 4, ' ', 0)
	if !quiet {
		fmt.Fprintln(w, "REPOSITORY\tTAG\tIMAGE ID\tCREATED\tSIZE")
	}

	for _, img := range imageList {
		size, err := img.Size(ctx, cs, platforms.DefaultStrict())
		if err != nil {
			return errors.Wrap(err, "failed to compute image size")
		}

		repository, tag := parseRepoTag(img.Name)

		var digest string
		if !noTrunc {
			digest = strings.Split(img.Target.Digest.String(), ":")[1][:12]
		} else {
			digest = img.Target.Digest.String()
		}

		if quiet {
			if _, err := fmt.Fprintf(w, "%s\n", digest); err != nil {
				return err
			}
			continue
		}

		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			repository,
			tag,
			digest,
			timeSinceInHuman(img.CreatedAt),
			progress.Bytes(size),
		); err != nil {
			return err
		}
	}
	return w.Flush()
}

func parseRepoTag(imgName string) (string, string) {
	logrus.Debugf("raw image name=%q", imgName)

	if strings.Contains(imgName, "@") {
		logrus.Warnf("unparsable image name %q", imgName)
		return "", ""
	}

	if _, err := refdocker.ParseDockerRef(imgName); err != nil {
		logrus.WithError(err).Warnf("unparsable image name %q", imgName)
		return "", ""
	}

	var tag string
	nameWithTagSplit := strings.Split(imgName, ":")
	if len(nameWithTagSplit) > 1 {
		tag = nameWithTagSplit[len(nameWithTagSplit)-1]
	}
	repository := strings.TrimSuffix(imgName, ":"+tag)
	repository = strings.TrimPrefix(repository, "docker.io/library/")
	repository = strings.TrimPrefix(repository, "docker.io/")

	return repository, tag
}
