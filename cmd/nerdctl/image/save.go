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
	"fmt"
	"io"
	"os"

	"github.com/containerd/containerd/images/archive"
	ncclient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/pkg/platformutil"
	"github.com/containerd/nerdctl/pkg/referenceutil"
	"github.com/mattn/go-isatty"

	"github.com/spf13/cobra"
)

func NewSaveCommand() *cobra.Command {
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
	saveCommand.RegisterFlagCompletionFunc("platform", completion.ShellCompletePlatforms)
	saveCommand.Flags().Bool("all-platforms", false, "Export content for all platforms")
	// #endregion

	return saveCommand
}

func saveAction(cmd *cobra.Command, args []string) error {
	var (
		images   = args
		saveOpts = []archive.ExportOpt{}
	)

	if len(images) == 0 {
		return fmt.Errorf("requires at least 1 argument")
	}

	out := cmd.OutOrStdout()
	output, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	}
	if output != "" {
		f, err := os.OpenFile(output, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	} else {
		if isatty.IsTerminal(os.Stdout.Fd()) {
			return fmt.Errorf("cowardly refusing to save to a terminal. Use the -o flag or redirect")
		}
	}
	return saveImage(images, out, saveOpts, cmd)
}

func saveImage(images []string, out io.Writer, saveOpts []archive.ExportOpt, cmd *cobra.Command) error {
	client, ctx, cancel, err := ncclient.New(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	allPlatforms, err := cmd.Flags().GetBool("all-platforms")
	if err != nil {
		return err
	}
	platform, err := cmd.Flags().GetStringSlice("platform")
	if err != nil {
		return err
	}
	platMC, err := platformutil.NewMatchComparer(allPlatforms, platform)
	if err != nil {
		return err
	}

	saveOpts = append(saveOpts, archive.WithPlatform(platMC))

	imageStore := client.ImageService()
	for _, img := range images {
		named, err := referenceutil.ParseAny(img)
		if err != nil {
			return err
		}
		saveOpts = append(saveOpts, archive.WithImage(imageStore, named.String()))
	}

	return client.Export(ctx, out, saveOpts...)
}

func saveShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show image names
	return completion.ShellCompleteImageNames(cmd)
}
