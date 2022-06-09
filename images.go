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
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"text/template"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/pkg/progress"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/referenceutil"
	"github.com/opencontainers/image-spec/identity"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newImagesCommand() *cobra.Command {
	shortHelp := "List images"
	longHelp := shortHelp + `

Properties:
- REPOSITORY: Repository
- TAG:        Tag
- IMAGE ID:   OCI Digest. Usually different from Docker image ID. Shared for multi-platform images.
- CREATED:    Created time
- PLATFORM:   Platform
- SIZE:       Size of the unpacked snapshots
- BLOB SIZE:  Size of the blobs (such as layer tarballs) in the content store
`
	var imagesCommand = &cobra.Command{
		Use:               "images",
		Short:             shortHelp,
		Long:              longHelp,
		Args:              cobra.MaximumNArgs(1),
		RunE:              imagesAction,
		ValidArgsFunction: imagesShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}

	imagesCommand.Flags().BoolP("quiet", "q", false, "Only show numeric IDs")
	imagesCommand.Flags().Bool("no-trunc", false, "Don't truncate output")
	// Alias "-f" is reserved for "--filter"
	imagesCommand.Flags().String("format", "", "Format the output using the given Go template, e.g, '{{json .}}', 'wide'")
	imagesCommand.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json", "table", "wide"}, cobra.ShellCompDirectiveNoFileComp
	})
	imagesCommand.Flags().Bool("digests", false, "Show digests (compatible with Docker, unlike ID)")
	imagesCommand.Flags().BoolP("all", "a", true, "(unimplemented yet, always true)")

	return imagesCommand
}

func imagesAction(cmd *cobra.Command, args []string) error {
	var filters []string

	if len(args) > 0 {
		canonicalRef, err := referenceutil.ParseAny(args[0])
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
	Digest       string // "<none>" or image target digest (i.e., index digest or manifest digest)
	ID           string // image target digest (not config digest, unlike Docker), or its short form
	Repository   string
	Tag          string // "<none>" or tag
	Size         string // the size of the unpacked snapshots.
	BlobSize     string // the size of the blobs in the content store (nerdctl extension)
	// TODO: "SharedSize", "UniqueSize", "VirtualSize"
	Platform string // nerdctl extension
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
	digestsFlag, err := cmd.Flags().GetBool("digests")
	if err != nil {
		return err
	}
	var w io.Writer
	w = os.Stdout
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	if format == "wide" {
		digestsFlag = true
	}
	var tmpl *template.Template
	switch format {
	case "", "table", "wide":
		w = tabwriter.NewWriter(w, 4, 8, 4, ' ', 0)
		if !quiet {
			if digestsFlag {
				fmt.Fprintln(w, "REPOSITORY\tTAG\tDIGEST\tIMAGE ID\tCREATED\tPLATFORM\tSIZE\tBLOB SIZE")
			} else {
				fmt.Fprintln(w, "REPOSITORY\tTAG\tIMAGE ID\tCREATED\tPLATFORM\tSIZE\tBLOB SIZE")
			}
		}
	case "raw":
		return errors.New("unsupported format: \"raw\"")
	default:
		if quiet {
			return errors.New("format and quiet must not be specified together")
		}
		var err error
		tmpl, err = parseTemplate(format)
		if err != nil {
			return err
		}
	}

	snapshotter, err := cmd.Flags().GetString("snapshotter")
	if err != nil {
		return err
	}

	printer := &imagePrinter{
		w:            w,
		quiet:        quiet,
		noTrunc:      noTrunc,
		digestsFlag:  digestsFlag,
		tmpl:         tmpl,
		client:       client,
		contentStore: client.ContentStore(),
		snapshotter:  client.SnapshotService(snapshotter),
	}

	for _, img := range imageList {
		if err := printer.printImage(ctx, img); err != nil {
			logrus.Warn(err)
		}
	}
	if f, ok := w.(Flusher); ok {
		return f.Flush()
	}
	return nil
}

type imagePrinter struct {
	w                           io.Writer
	quiet, noTrunc, digestsFlag bool
	tmpl                        *template.Template
	client                      *containerd.Client
	contentStore                content.Store
	snapshotter                 snapshots.Snapshotter
}

func (x *imagePrinter) printImage(ctx context.Context, img images.Image) error {
	ociPlatforms, err := images.Platforms(ctx, x.contentStore, img.Target)
	if err != nil {
		logrus.WithError(err).Warnf("failed to get the platform list of image %q", img.Name)
		return x.printImageSinglePlatform(ctx, img, platforms.DefaultSpec())
	}
	for _, ociPlatform := range ociPlatforms {
		if err := x.printImageSinglePlatform(ctx, img, ociPlatform); err != nil {
			logrus.WithError(err).Warnf("failed to get platform %q of image %q", platforms.Format(ociPlatform), img.Name)
		}
	}
	return nil
}

func (x *imagePrinter) printImageSinglePlatform(ctx context.Context, img images.Image, ociPlatform v1.Platform) error {
	platMC := platforms.OnlyStrict(ociPlatform)
	if avail, _, _, _, availErr := images.Check(ctx, x.contentStore, img.Target, platMC); !avail {
		logrus.WithError(availErr).Debugf("skipping printing image %q for platform %q", img.Name, platforms.Format(ociPlatform))
		return nil
	}

	blobSize, err := img.Size(ctx, x.contentStore, platMC)
	if err != nil {
		logrus.WithError(err).Warnf("failed to get blob size of image %q for platform %q", img.Name, platforms.Format(ociPlatform))
	}

	size, err := unpackedImageSize(ctx, x.client, x.snapshotter, img, platMC)
	if err != nil {
		logrus.WithError(err).Warnf("failed to get unpacked size of image %q for platform %q", img.Name, platforms.Format(ociPlatform))
	}

	repository, tag := imgutil.ParseRepoTag(img.Name)

	p := imagePrintable{
		CreatedAt:    img.CreatedAt.Round(time.Second).Local().String(), // format like "2021-08-07 02:19:45 +0900 JST"
		CreatedSince: formatter.TimeSinceInHuman(img.CreatedAt),
		Digest:       img.Target.Digest.String(),
		ID:           img.Target.Digest.String(),
		Repository:   repository,
		Tag:          tag,
		Size:         progress.Bytes(size).String(),
		BlobSize:     progress.Bytes(blobSize).String(),
		Platform:     platforms.Format(ociPlatform),
	}
	if p.Tag == "" {
		p.Tag = "<none>" // for Docker compatibility
	}
	if !x.noTrunc {
		// p.Digest does not need to be truncated
		p.ID = strings.Split(p.ID, ":")[1][:12]
	}
	if x.tmpl != nil {
		var b bytes.Buffer
		if err := x.tmpl.Execute(&b, p); err != nil {
			return err
		}
		if _, err = fmt.Fprintf(x.w, b.String()+"\n"); err != nil {
			return err
		}
	} else if x.quiet {
		if _, err := fmt.Fprintf(x.w, "%s\n", p.ID); err != nil {
			return err
		}
	} else {
		if x.digestsFlag {
			if _, err := fmt.Fprintf(x.w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				p.Repository,
				p.Tag,
				p.Digest,
				p.ID,
				p.CreatedSince,
				p.Platform,
				p.Size,
				p.BlobSize,
			); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(x.w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				p.Repository,
				p.Tag,
				p.ID,
				p.CreatedSince,
				p.Platform,
				p.Size,
				p.BlobSize,
			); err != nil {
				return err
			}
		}
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

// unpackedImageSize is the size of the unpacked snapshots.
// Does not contain the size of the blobs in the content store. (Corresponds to Docker).
func unpackedImageSize(ctx context.Context, client *containerd.Client, s snapshots.Snapshotter, i images.Image, platMC platforms.MatchComparer) (int64, error) {
	img := containerd.NewImageWithPlatform(client, i, platMC)

	diffIDs, err := img.RootFS(ctx)
	if err != nil {
		return 0, err
	}

	chainID := identity.ChainID(diffIDs).String()
	usage, err := s.Usage(ctx, chainID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			logrus.WithError(err).Debugf("image %q seems not unpacked", i.Name)
			return 0, nil
		}
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
