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
	"io"
	"os"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/images/archive"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/idutil/imagewalker"
	"github.com/containerd/nerdctl/pkg/platformutil"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

// SaveOptions contain options used by `nerdctl save`.
type SaveOptions struct {
	AllPlatforms bool
	Output       string
	Platform     []string
}

func newSaveCommand() *cobra.Command {
	var saveCommand = &cobra.Command{
		Use:               "save",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Save one or more images to a tar archive (streamed to STDOUT by default)",
		Long:              "The archive implements both Docker Image Spec v1.2 and OCI Image Spec v1.0.",
		RunE:              saveAction,
		ValidArgsFunction: saveShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	saveCommand.Flags().StringP("output", "o", "", "Write to a file, instead of STDOUT")

	// #region platform flags
	// platform is defined as StringSlice, not StringArray, to allow specifying "--platform=amd64,arm64"
	saveCommand.Flags().StringSlice("platform", []string{}, "Export content for a specific platform")
	saveCommand.RegisterFlagCompletionFunc("platform", shellCompletePlatforms)
	saveCommand.Flags().Bool("all-platforms", false, "Export content for all platforms")
	// #endregion

	return saveCommand
}

func saveAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("requires at least 1 argument")
	}

	output, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	}
	allPlatforms, err := cmd.Flags().GetBool("all-platforms")
	if err != nil {
		return err
	}
	platform, err := cmd.Flags().GetStringSlice("platform")
	if err != nil {
		return err
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	opt := SaveOptions{
		AllPlatforms: allPlatforms,
		Output:       output,
		Platform:     platform,
	}

	writer := cmd.OutOrStdout()
	if output != "" {
		f, err := os.OpenFile(output, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
		writer = f
	} else {
		if isatty.IsTerminal(os.Stdout.Fd()) {
			return fmt.Errorf("cowardly refusing to save to a terminal. Use the -o flag or redirect")
		}
	}
	return saveImages(ctx, client, args, writer, opt)
}

func saveImages(ctx context.Context, client *containerd.Client, images []string, writer io.Writer, opt SaveOptions, exportOpts ...archive.ExportOpt) error {
	images = strutil.DedupeStrSlice(images)

	platMC, err := platformutil.NewMatchComparer(opt.AllPlatforms, opt.Platform)
	if err != nil {
		return err
	}

	exportOpts = append(exportOpts, archive.WithPlatform(platMC))
	imageStore := client.ImageService()

	savedImages := make(map[string]struct{})
	walker := &imagewalker.ImageWalker{
		Client: client,
		OnFound: func(ctx context.Context, found imagewalker.Found) error {
			if found.UniqueImages > 1 {
				return fmt.Errorf("ambiguous digest ID: multiple IDs found with provided prefix %s", found.Req)
			}
			imgName := found.Image.Name
			imgDigest := found.Image.Target.Digest.String()
			if _, ok := savedImages[imgDigest]; !ok {
				savedImages[imgDigest] = struct{}{}
				exportOpts = append(exportOpts, archive.WithImage(imageStore, imgName))
			}
			return nil
		},
	}

	for _, img := range images {
		count, err := walker.Walk(ctx, img)
		if err != nil {
			return err
		}
		if count == 0 {
			return fmt.Errorf("no such image: %s", img)
		}
	}
	return client.Export(ctx, writer, exportOpts...)
}

func saveShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show image names
	return shellCompleteImageNames(cmd)
}
