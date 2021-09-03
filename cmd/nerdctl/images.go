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
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"text/template"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/pkg/progress"
	"github.com/containerd/containerd/platforms"
	refdocker "github.com/containerd/containerd/reference/docker"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/docker/cli/templates"
	"github.com/opencontainers/image-spec/identity"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newImagesCommand() *cobra.Command {
	var imagesCommand = &cobra.Command{
		Use:               "images",
		Short:             "List images",
		Args:              cobra.MaximumNArgs(1),
		RunE:              imagesAction,
		ValidArgsFunction: imagesShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}

	imagesCommand.Flags().BoolP("quiet", "q", false, "Only show numeric IDs")
	imagesCommand.Flags().Bool("no-trunc", false, "Don't truncate output")
	// Alias "-f" is reserved for "--filter"
	imagesCommand.Flags().String("format", "", "Format the output using the given Go template, e.g, '{{json .}}'")

	return imagesCommand
}

func imagesAction(cmd *cobra.Command, args []string) error {
	var filters []string

	if len(args) > 1 {
		return errors.New("cannot have more than one argument")
	}

	if len(args) > 0 {
		canonicalRef, err := refdocker.ParseDockerRef(args[0])
		if err != nil {
			return err
		}
		filters = append(filters, fmt.Sprintf("name==%s", canonicalRef.String()))
	}
	client, ctx, cancel, err := newClient(cmd)
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

	return printImages(ctx, cmd, client, imageList, cs)
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

func printImages(ctx context.Context, cmd *cobra.Command, client *containerd.Client, imageList []images.Image, cs content.Store) error {
	quiet, err := cmd.Flags().GetBool("quiet")
	if err != nil {
		return err
	}
	noTrunc, err := cmd.Flags().GetBool("no-trunc")
	if err != nil {
		return err
	}
	var w io.Writer
	w = os.Stdout
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	var tmpl *template.Template
	switch format {
	case "", "table":
		w = tabwriter.NewWriter(w, 4, 8, 4, ' ', 0)
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

	snapshotter, err := cmd.Flags().GetString("snapshotter")
	if err != nil {
		return err
	}
	s := client.SnapshotService(snapshotter)

	var errs []error
	for _, img := range imageList {
		size, err := unpackedImageSize(ctx, cmd, client, s, img)
		if err != nil {
			errs = append(errs, err)
		}
		repository, tag := imgutil.ParseRepoTag(img.Name)

		config, err := img.Config(ctx, cs, platforms.DefaultStrict())
		if err != nil {
			return err
		}

		p := imagePrintable{
			CreatedAt:    img.CreatedAt.Round(time.Second).Local().String(), // format like "2021-08-07 02:19:45 +0900 JST"
			CreatedSince: formatter.TimeSinceInHuman(img.CreatedAt),
			ID:           config.Digest.String(),
			Repository:   repository,
			Tag:          tag,
			Size:         progress.Bytes(size).String(),
		}
		if !noTrunc {
			p.ID = strings.Split(config.Digest.String(), ":")[1][:12]
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

func imagesShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		// show image names
		return shellCompleteImageNames(cmd)
	} else {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
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

func unpackedImageSize(ctx context.Context, cmd *cobra.Command, client *containerd.Client, s snapshots.Snapshotter, i images.Image) (int64, error) {
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
