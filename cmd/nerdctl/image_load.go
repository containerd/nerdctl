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
	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/image"
)

func newLoadCommand() *cobra.Command {
	var loadCommand = &cobra.Command{
		Use:           "load",
		Args:          cobra.NoArgs,
		Short:         "Load an image from a tar archive or STDIN",
		Long:          "Supports both Docker Image Spec v1.2 and OCI Image Spec v1.0.",
		RunE:          loadAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	loadCommand.Flags().StringP("input", "i", "", "Read from tar archive file, instead of STDIN")

	// #region platform flags
	// platform is defined as StringSlice, not StringArray, to allow specifying "--platform=amd64,arm64"
	loadCommand.Flags().StringSlice("platform", []string{}, "Import content for a specific platform")
	loadCommand.RegisterFlagCompletionFunc("platform", completion.Platforms)
	loadCommand.Flags().Bool("all-platforms", false, "Import content for all platforms")
	// #endregion

	return loadCommand
}

func processLoadCommandFlags(cmd *cobra.Command) (types.ImageLoadOptions, error) {
	input, err := cmd.Flags().GetString("input")
	if err != nil {
		return types.ImageLoadOptions{}, err
	}
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.ImageLoadOptions{}, err
	}
	allPlatforms, err := cmd.Flags().GetBool("all-platforms")
	if err != nil {
		return types.ImageLoadOptions{}, err
	}
	platform, err := cmd.Flags().GetStringSlice("platform")
	if err != nil {
		return types.ImageLoadOptions{}, err
	}
	return types.ImageLoadOptions{
		GOptions:     globalOptions,
		Input:        input,
		Platform:     platform,
		AllPlatforms: allPlatforms,
		Stdout:       cmd.OutOrStdout(),
		Stdin:        cmd.InOrStdin(),
	}, nil
}

func loadAction(cmd *cobra.Command, _ []string) error {
	options, err := processLoadCommandFlags(cmd)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return image.Load(ctx, client, options)
}
