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
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/tabwriter"
	"text/template"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/pkg/progress"
	refdocker "github.com/containerd/containerd/reference/docker"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/docker/cli/templates"
	"github.com/opencontainers/image-spec/identity"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var imagesCommand = &cli.Command{
	Name:         "images",
	Usage:        "List images",
	Action:       imagesAction,
	BashComplete: imagesBashComplete,
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
		&cli.StringFlag{
			Name: "format",
			// Alias "-f" is reserved for "--filter"
			Usage: "Format the output using the given Go template, e.g, '{{json .}}'",
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

	return printImages(ctx, clicontext, client, imageList, cs)
}

type imagePrintable struct {
	// TODO: "Containers"
	CreatedAt    string
	CreatedSince string
	// TODO: "Digest" (only when --digests is set)
	ID         string
	Repository string
	Tag        string
	Size       string
	// TODO: "SharedSize", "UniqueSize", "VirtualSize"
}

func printImages(ctx context.Context, clicontext *cli.Context, client *containerd.Client, imageList []images.Image, cs content.Store) error {
	quiet := clicontext.Bool("quiet")
	noTrunc := clicontext.Bool("no-trunc")
	w := clicontext.App.Writer
	var tmpl *template.Template
	switch format := clicontext.String("format"); format {
	case "", "table":
		w = tabwriter.NewWriter(clicontext.App.Writer, 4, 8, 4, ' ', 0)
		if !quiet {
			fmt.Fprintln(w, "REPOSITORY\tTAG\tIMAGE ID\tCREATED\tSIZE")
		}
	case "raw":
		return errors.New("unsupported format: \"raw\"")
	default:
		if quiet {
			return errors.New("format and quiet must not be specified together")
		}
		var err error
		tmpl, err = templates.Parse(format)
		if err != nil {
			return err
		}
	}

	s := client.SnapshotService(clicontext.String("snapshotter"))

	var errs []error
	for _, img := range imageList {
		size, err := unpackedImageSize(ctx, clicontext, client, s, img)
		if err != nil {
			errs = append(errs, err)
		}
		repository, tag := imgutil.ParseRepoTag(img.Name)

		p := imagePrintable{
			CreatedAt:    img.CreatedAt.Round(time.Second).Local().String(), // format like "2021-08-07 02:19:45 +0900 JST"
			CreatedSince: timeSinceInHuman(img.CreatedAt),
			ID:           img.Target.Digest.String(),
			Repository:   repository,
			Tag:          tag,
			Size:         progress.Bytes(size).String(),
		}
		if !noTrunc {
			p.ID = strings.Split(img.Target.Digest.String(), ":")[1][:12]
		}
		if tmpl != nil {
			var b bytes.Buffer
			if err := tmpl.Execute(&b, p); err != nil {
				return err
			}
			if _, err = fmt.Fprintf(w, b.String()+"\n"); err != nil {
				return err
			}
		} else if quiet {
			if _, err := fmt.Fprintf(w, "%s\n", p.ID); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				repository,
				tag,
				p.ID,
				p.CreatedSince,
				p.Size,
			); err != nil {
				return err
			}
		}
	}
	if len(errs) > 0 {
		logrus.Warn("failed to compute image(s) size")
	}
	if f, ok := w.(Flusher); ok {
		return f.Flush()
	}
	return nil
}

func imagesBashComplete(clicontext *cli.Context) {
	coco := parseCompletionContext(clicontext)
	if coco.boring || coco.flagTakesValue {
		defaultBashComplete(clicontext)
		return
	}
	// show image names
	bashCompleteImageNames(clicontext)
}

type snapshotKey string

// recursive function to calculate total usage of key's parent
func (key snapshotKey) add(ctx context.Context, s snapshots.Snapshotter, usage *snapshots.Usage) error {
	if key == "" {
		return nil
	}
	u, err := s.Usage(ctx, string(key))
	if err != nil {
		return err
	}

	usage.Add(u)

	info, err := s.Stat(ctx, string(key))
	if err != nil {
		return err
	}

	key = snapshotKey(info.Parent)
	return key.add(ctx, s, usage)
}

func unpackedImageSize(ctx context.Context, clicontext *cli.Context, client *containerd.Client, s snapshots.Snapshotter, i images.Image) (int64, error) {
	img := containerd.NewImage(client, i)

	diffIDs, err := img.RootFS(ctx)
	if err != nil {
		return 0, err
	}

	chainID := identity.ChainID(diffIDs).String()
	usage, err := s.Usage(ctx, chainID)
	if err != nil {
		return 0, err
	}

	info, err := s.Stat(ctx, chainID)
	if err != nil {
		return 0, err
	}

	//add ChainID's parent usage to the total usage
	if err := snapshotKey(info.Parent).add(ctx, s, &usage); err != nil {
		return 0, err
	}
	return usage.Size, nil
}
