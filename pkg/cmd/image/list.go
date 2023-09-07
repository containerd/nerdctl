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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"text/template"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/pkg/progress"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/imgutil"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
)

// ListCommandHandler `List` and print images matching filters in `options`.
func ListCommandHandler(ctx context.Context, client *containerd.Client, options types.ImageListOptions) error {
	imageList, err := List(ctx, client, options.Filters, options.NameAndRefFilter)
	if err != nil {
		return err
	}
	return printImages(ctx, client, imageList, options)
}

// List queries containerd client to get image list and only returns those matching given filters.
//
// Supported filters:
// - before=<image>[:<tag>]: Images created before given image (exclusive)
// - since=<image>[:<tag>]: Images created after given image (exclusive)
// - label=<key>[=<value>]: Matches images based on the presence of a label alone or a label and a value
// - dangling=true: Filter images by dangling
// - reference=<image>[:<tag>]: Filter images by reference (Matches both docker compatible wildcard pattern and regexp
//
// nameAndRefFilter has the format of `name==(<image>[:<tag>])|ID`,
// and they will be used when getting images from containerd,
// while the remaining filters are only applied after getting images from containerd,
// which means that having nameAndRefFilter may speed up the process if there are a lot of images in containerd.
func List(ctx context.Context, client *containerd.Client, filters, nameAndRefFilter []string) ([]images.Image, error) {
	var imageStore = client.ImageService()
	imageList, err := imageStore.List(ctx, nameAndRefFilter...)
	if err != nil {
		return nil, err
	}
	if len(filters) > 0 {
		f, err := imgutil.ParseFilters(filters)
		if err != nil {
			return nil, err
		}

		if f.Dangling != nil {
			imageList = imgutil.FilterDangling(imageList, *f.Dangling)
		}

		imageList, err = imgutil.FilterByLabel(ctx, client, imageList, f.Labels)
		if err != nil {
			return nil, err
		}

		imageList, err = imgutil.FilterByReference(imageList, f.Reference)
		if err != nil {
			return nil, err
		}

		var beforeImages []images.Image
		if len(f.Before) > 0 {
			beforeImages, err = imageStore.List(ctx, f.Before...)
			if err != nil {
				return nil, err
			}
		}
		var sinceImages []images.Image
		if len(f.Since) > 0 {
			sinceImages, err = imageStore.List(ctx, f.Since...)
			if err != nil {
				return nil, err
			}
		}

		imageList = imgutil.FilterImages(imageList, beforeImages, sinceImages)
	}
	return imageList, nil
}

type imagePrintable struct {
	// TODO: "Containers"
	CreatedAt    string
	CreatedSince string
	Digest       string // "<none>" or image target digest (i.e., index digest or manifest digest)
	ID           string // image target digest (not config digest, unlike Docker), or its short form
	Repository   string
	Tag          string // "<none>" or tag
	Name         string // image name
	Size         string // the size of the unpacked snapshots.
	BlobSize     string // the size of the blobs in the content store (nerdctl extension)
	// TODO: "SharedSize", "UniqueSize"
	Platform string // nerdctl extension
}

func printImages(ctx context.Context, client *containerd.Client, imageList []images.Image, options types.ImageListOptions) error {
	w := options.Stdout
	digestsFlag := options.Digests
	if options.Format == "wide" {
		digestsFlag = true
	}
	var tmpl *template.Template
	switch options.Format {
	case "", "table", "wide":
		w = tabwriter.NewWriter(w, 4, 8, 4, ' ', 0)
		if !options.Quiet {
			printHeader := ""
			if options.Names {
				printHeader += "NAME\t"
			} else {
				printHeader += "REPOSITORY\tTAG\t"
			}
			if digestsFlag {
				printHeader += "DIGEST\t"
			}
			printHeader += "IMAGE ID\tCREATED\tPLATFORM\tSIZE\tBLOB SIZE"
			fmt.Fprintln(w, printHeader)
		}
	case "raw":
		return errors.New("unsupported format: \"raw\"")
	default:
		if options.Quiet {
			return errors.New("format and quiet must not be specified together")
		}
		var err error
		tmpl, err = formatter.ParseTemplate(options.Format)
		if err != nil {
			return err
		}
	}

	printer := &imagePrinter{
		w:            w,
		quiet:        options.Quiet,
		noTrunc:      options.NoTrunc,
		digestsFlag:  digestsFlag,
		namesFlag:    options.Names,
		tmpl:         tmpl,
		client:       client,
		contentStore: client.ContentStore(),
		snapshotter:  client.SnapshotService(options.GOptions.Snapshotter),
	}

	for _, img := range imageList {
		if err := printer.printImage(ctx, img); err != nil {
			logrus.Warn(err)
		}
	}
	if f, ok := w.(formatter.Flusher); ok {
		return f.Flush()
	}
	return nil
}

type imagePrinter struct {
	w                                      io.Writer
	quiet, noTrunc, digestsFlag, namesFlag bool
	tmpl                                   *template.Template
	client                                 *containerd.Client
	contentStore                           content.Store
	snapshotter                            snapshots.Snapshotter
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

	image := containerd.NewImageWithPlatform(x.client, img, platMC)
	desc, err := image.Config(ctx)
	if err != nil {
		logrus.WithError(err).Warnf("failed to get config of image %q for platform %q", img.Name, platforms.Format(ociPlatform))
	}
	var (
		repository string
		tag        string
	)
	// cri plugin will create an image named digest of image's config, skip parsing.
	if x.namesFlag || desc.Digest.String() != img.Name {
		repository, tag = imgutil.ParseRepoTag(img.Name)
	}

	blobSize, err := image.Size(ctx)
	if err != nil {
		logrus.WithError(err).Warnf("failed to get blob size of image %q for platform %q", img.Name, platforms.Format(ociPlatform))
	}

	size, err := imgutil.UnpackedImageSize(ctx, x.snapshotter, image)
	if err != nil {
		// Warnf is too verbose: https://github.com/containerd/nerdctl/issues/2058
		logrus.WithError(err).Debugf("failed to get unpacked size of image %q for platform %q", img.Name, platforms.Format(ociPlatform))
	}

	p := imagePrintable{
		CreatedAt:    img.CreatedAt.Round(time.Second).Local().String(), // format like "2021-08-07 02:19:45 +0900 JST"
		CreatedSince: formatter.TimeSinceInHuman(img.CreatedAt),
		Digest:       img.Target.Digest.String(),
		ID:           img.Target.Digest.String(),
		Repository:   repository,
		Tag:          tag,
		Name:         img.Name,
		Size:         progress.Bytes(size).String(),
		BlobSize:     progress.Bytes(blobSize).String(),
		Platform:     platforms.Format(ociPlatform),
	}
	if p.Repository == "" {
		p.Repository = "<none>"
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
		if _, err = fmt.Fprintln(x.w, b.String()); err != nil {
			return err
		}
	} else if x.quiet {
		if _, err := fmt.Fprintln(x.w, p.ID); err != nil {
			return err
		}
	} else {
		format := ""
		args := []interface{}{}
		if x.namesFlag {
			format += "%s\t"
			args = append(args, p.Name)
		} else {
			format += "%s\t%s\t"
			args = append(args, p.Repository, p.Tag)
		}
		if x.digestsFlag {
			format += "%s\t"
			args = append(args, p.Digest)
		}

		format += "%s\t%s\t%s\t%s\t%s\n"
		args = append(args, p.ID, p.CreatedSince, p.Platform, p.Size, p.BlobSize)
		if _, err := fmt.Fprintf(x.w, format, args...); err != nil {
			return err
		}
	}
	return nil
}
