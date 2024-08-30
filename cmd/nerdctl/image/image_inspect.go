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
	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/image"
)

func newImageInspectCommand() *cobra.Command {
	var imageInspectCommand = &cobra.Command{
		Use:               "inspect [flags] IMAGE [IMAGE...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Display detailed information on one or more images.",
		Long:              "Hint: set `--mode=native` for showing the full output",
		RunE:              imageInspectAction,
		ValidArgsFunction: imageInspectShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	imageInspectCommand.Flags().String("mode", "dockercompat", `Inspect mode, "dockercompat" for Docker-compatible output, "native" for containerd-native output`)
	imageInspectCommand.RegisterFlagCompletionFunc("mode", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"dockercompat", "native"}, cobra.ShellCompDirectiveNoFileComp
	})
	imageInspectCommand.Flags().StringP("format", "f", "", "Format the output using the given Go template, e.g, '{{json .}}'")
	imageInspectCommand.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json"}, cobra.ShellCompDirectiveNoFileComp
	})

	// #region platform flags
	imageInspectCommand.Flags().String("platform", "", "Inspect a specific platform") // not a slice, and there is no --all-platforms
	imageInspectCommand.RegisterFlagCompletionFunc("platform", completion.Platforms)
	// #endregion

	return imageInspectCommand
}

func ProcessImageInspectOptions(cmd *cobra.Command, platform *string) (types.ImageInspectOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.ImageInspectOptions{}, err
	}
	mode, err := cmd.Flags().GetString("mode")
	if err != nil {
		return types.ImageInspectOptions{}, err
	}
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return types.ImageInspectOptions{}, err
	}
	if platform == nil {
		tempPlatform, err := cmd.Flags().GetString("platform")
		if err != nil {
			return types.ImageInspectOptions{}, err
		}
		platform = &tempPlatform
	}
	return types.ImageInspectOptions{
		GOptions: globalOptions,
		Mode:     mode,
		Format:   format,
		Platform: *platform,
		Stdout:   cmd.OutOrStdout(),
	}, nil
}

func imageInspectAction(cmd *cobra.Command, args []string) error {
	options, err := ProcessImageInspectOptions(cmd, nil)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClientWithPlatform(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address, options.Platform)
	if err != nil {
		return err
	}
	defer cancel()

	return image.Inspect(ctx, client, args, options)
}

func imageInspectShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show image names
	return completion.ImageNames(cmd)
}
