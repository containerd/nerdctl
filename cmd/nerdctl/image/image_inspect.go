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

	"github.com/spf13/cobra"

	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/image"
	"github.com/containerd/nerdctl/v2/pkg/formatter"
)

func inspectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "inspect [flags] IMAGE [IMAGE...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Display detailed information on one or more images.",
		Long:              "Hint: set `--mode=native` for showing the full output",
		RunE:              imageInspectAction,
		ValidArgsFunction: imageInspectShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	cmd.Flags().String("mode", "dockercompat", `Inspect mode, "dockercompat" for Docker-compatible output, "native" for containerd-native output`)
	cmd.RegisterFlagCompletionFunc("mode", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"dockercompat", "native"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().StringP("format", "f", "", "Format the output using the given Go template, e.g, '{{json .}}'")
	cmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json"}, cobra.ShellCompDirectiveNoFileComp
	})

	// #region platform flags
	cmd.Flags().String("platform", "", "Inspect a specific platform") // not a slice, and there is no --all-platforms
	cmd.RegisterFlagCompletionFunc("platform", completion.Platforms)
	// #endregion

	return cmd
}

func InspectOptions(cmd *cobra.Command, platform *string) (types.ImageInspectOptions, error) {
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
	options, err := InspectOptions(cmd, nil)
	if err != nil {
		return err
	}

	// Verify we have a valid mode
	if options.Mode != "native" && options.Mode != "dockercompat" {
		return fmt.Errorf("unknown mode %q", options.Mode)
	}

	client, ctx, cancel, err := clientutil.NewClientWithPlatform(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address, options.Platform)
	if err != nil {
		return err
	}
	defer cancel()

	entries, err := image.Inspect(ctx, client, args, options)
	if err != nil {
		return err
	}

	// Display
	if len(entries) > 0 {
		if formatErr := formatter.FormatSlice(options.Format, options.Stdout, entries); formatErr != nil {
			log.G(ctx).Error(formatErr)
		}
	}
	return err
}

func imageInspectShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show image names
	return completion.ImageNames(cmd)
}
