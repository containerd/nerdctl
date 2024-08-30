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
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/image"
)

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
	saveCommand.RegisterFlagCompletionFunc("platform", completion.Platforms)
	saveCommand.Flags().Bool("all-platforms", false, "Export content for all platforms")
	// #endregion

	return saveCommand
}

func processImageSaveOptions(cmd *cobra.Command) (types.ImageSaveOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.ImageSaveOptions{}, err
	}

	allPlatforms, err := cmd.Flags().GetBool("all-platforms")
	if err != nil {
		return types.ImageSaveOptions{}, err
	}
	platform, err := cmd.Flags().GetStringSlice("platform")
	if err != nil {
		return types.ImageSaveOptions{}, err
	}

	return types.ImageSaveOptions{
		GOptions:     globalOptions,
		AllPlatforms: allPlatforms,
		Platform:     platform,
	}, err
}

func saveAction(cmd *cobra.Command, args []string) error {
	options, err := processImageSaveOptions(cmd)
	if err != nil {
		return err
	}

	output := cmd.OutOrStdout()
	outputPath, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	} else if outputPath != "" {
		f, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		output = f
		defer f.Close()
	} else if out, ok := output.(*os.File); ok && isatty.IsTerminal(out.Fd()) {
		return fmt.Errorf("cowardly refusing to save to a terminal. Use the -o flag or redirect")
	}
	options.Stdout = output

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	if err = image.Save(ctx, client, args, options); err != nil && outputPath != "" {
		os.Remove(outputPath)
	}
	return err
}

func saveShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show image names
	return completion.ImageNames(cmd)
}
