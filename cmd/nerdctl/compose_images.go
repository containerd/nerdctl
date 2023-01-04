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

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/pkg/progress"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

func newComposeImagesCommand() *cobra.Command {
	var composeImagesCommand = &cobra.Command{
		Use:           "images [flags] [SERVICE...]",
		Short:         "List images used by created containers in services",
		RunE:          composeImagesAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	composeImagesCommand.Flags().BoolP("quiet", "q", false, "Only show numeric image IDs")

	return composeImagesCommand
}

func composeImagesAction(cmd *cobra.Command, args []string) error {
	quiet, err := cmd.Flags().GetBool("quiet")
	if err != nil {
		return err
	}
	globalOptions, err := processGlobalFlag(cmd)
	if err != nil {
		return err
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	c, err := getComposer(cmd, client, globalOptions)
	if err != nil {
		return err
	}

	serviceNames, err := c.ServiceNames(args...)
	if err != nil {
		return err
	}

	containers, err := c.Containers(ctx, serviceNames...)
	if err != nil {
		return err
	}

	if quiet {
		return printComposeImageIDs(ctx, containers)
	}

	snapshotter, err := cmd.Flags().GetString("snapshotter")
	if err != nil {
		return err
	}
	sn := client.SnapshotService(snapshotter)

	return printComposeImages(ctx, cmd, containers, sn)
}

func printComposeImageIDs(ctx context.Context, containers []containerd.Container) error {
	ids := []string{}
	for _, c := range containers {
		image, err := c.Image(ctx)
		if err != nil {
			return err
		}
		metaImage := image.Metadata()
		id := metaImage.Target.Digest.String()
		if !strutil.InStringSlice(ids, id) {
			ids = append(ids, id)
		}
	}

	for _, id := range ids {
		// always truncate image ids.
		fmt.Println(strings.Split(id, ":")[1][:12])
	}
	return nil
}

func printComposeImages(ctx context.Context, cmd *cobra.Command, containers []containerd.Container, sn snapshots.Snapshotter) error {
	type composeImagePrintable struct {
		ContainerName string
		Repository    string
		Tag           string
		ImageID       string
		Size          string
	}

	imagePrintables := make([]composeImagePrintable, len(containers))
	eg, ctx := errgroup.WithContext(ctx)
	for i, c := range containers {
		i, c := i, c
		eg.Go(func() error {
			info, err := c.Info(ctx, containerd.WithoutRefreshedMetadata)
			if err != nil {
				return err
			}
			containerName := info.Labels[labels.Name]

			image, err := c.Image(ctx)
			if err != nil {
				return err
			}

			size, err := unpackedImageSize(ctx, sn, image)
			if err != nil {
				return err
			}

			metaImage := image.Metadata()
			repository, tag := imgutil.ParseRepoTag(metaImage.Name)
			imageID := metaImage.Target.Digest.String()
			if repository == "" {
				repository = "<none>"
			}
			if tag == "" {
				tag = "<none>"
			}
			imageID = strings.Split(imageID, ":")[1][:12]

			// no race condition since each goroutine accesses different `i`
			imagePrintables[i] = composeImagePrintable{
				ContainerName: containerName,
				Repository:    repository,
				Tag:           tag,
				ImageID:       imageID,
				Size:          progress.Bytes(size).String(),
			}

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return err
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 4, 8, 4, ' ', 0)
	fmt.Fprintln(w, "Container\tRepository\tTag\tImage Id\tSize")
	for _, p := range imagePrintables {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			p.ContainerName,
			p.Repository,
			p.Tag,
			p.ImageID,
			p.Size,
		); err != nil {
			return err
		}
	}

	return w.Flush()
}
