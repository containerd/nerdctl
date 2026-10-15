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

package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/pkg/progress"
	"github.com/containerd/platforms"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/compose"
	imagecmd "github.com/containerd/nerdctl/v2/pkg/cmd/image"
	"github.com/containerd/nerdctl/v2/pkg/formatter"
	"github.com/containerd/nerdctl/v2/pkg/imgutil"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
)

func imagesCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:           "images [flags] [SERVICE...]",
		Short:         "List images used by created containers in services",
		RunE:          imagesAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.Flags().String("format", "", "Format the output. Supported values: [json]")
	cmd.Flags().BoolP("quiet", "q", false, "Only show numeric image IDs")
	return cmd
}

func imagesAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return err
	}

	quiet, err := cmd.Flags().GetBool("quiet")
	if err != nil {
		return err
	}
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	if format != "json" && format != "" {
		return fmt.Errorf("unsupported format %s, supported formats are: [json]", format)
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	options, err := getComposeOptions(cmd, globalOptions.DebugFull, globalOptions.Experimental)
	if err != nil {
		return err
	}
	c, err := compose.New(client, globalOptions, options, cmd.OutOrStdout(), cmd.ErrOrStderr())
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

	sn := client.SnapshotService(globalOptions.Snapshotter)

	return printComposeImages(ctx, cmd, client, containers, sn, format)
}

func printComposeImageIDs(ctx context.Context, containers []containerd.Container) error {
	ids := []string{}
	for _, c := range containers {
		info, err := c.Info(ctx, containerd.WithoutRefreshedMetadata)
		if err != nil {
			return err
		}
		dgst, ok := imagecmd.PinnedImageDigest(info.Labels)
		if !ok {
			image, err := c.Image(ctx)
			if err != nil {
				return err
			}
			dgst = image.Target().Digest
		}
		id := dgst.String()
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

func printComposeImages(ctx context.Context, cmd *cobra.Command, client *containerd.Client, containers []containerd.Container, sn snapshots.Snapshotter, format string) error {
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

			image, err := composeContainerImage(ctx, client, c, info)
			if err != nil {
				return err
			}

			size, err := imgutil.UnpackedImageSize(ctx, sn, image)
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
			if format != "json" {
				imageID = strings.Split(imageID, ":")[1][:12]
			}

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

	if format == "json" {
		outJSON, err := formatter.ToJSON(imagePrintables, "", "")
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(cmd.OutOrStdout(), outJSON)
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

// composeContainerImage resolves the content pinned at container creation, so the
// image ID and unpacked size cannot follow a tag onto a different image.
func composeContainerImage(ctx context.Context, client *containerd.Client, c containerd.Container, info containers.Container) (containerd.Image, error) {
	dgst, ok := imagecmd.PinnedImageDigest(info.Labels)
	if !ok {
		return c.Image(ctx)
	}

	desc := ocispec.Descriptor{Digest: dgst}
	blob, err := content.ReadBlob(ctx, client.ContentStore(), desc)
	if err != nil {
		return nil, err
	}
	var index ocispec.Index
	if err := json.Unmarshal(blob, &index); err != nil {
		return nil, err
	}
	desc.MediaType = index.MediaType
	// OCI manifests and indexes may omit their mediaType field.
	if desc.MediaType == "" {
		desc.MediaType = ocispec.MediaTypeImageManifest
		if index.Manifests != nil {
			desc.MediaType = ocispec.MediaTypeImageIndex
		}
	}
	desc.Size = int64(len(blob))

	platform := platforms.DefaultSpec()
	if value := info.Labels[labels.Platform]; value != "" {
		platform, err = platforms.Parse(value)
		if err != nil {
			return nil, err
		}
	}
	metadata := images.Image{Name: info.Image, Target: desc}
	return containerd.NewImageWithPlatform(client, metadata, platforms.OnlyStrict(platform)), nil
}
