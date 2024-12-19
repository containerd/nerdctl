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

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/image"
)

func addSquashFlags(cmd *cobra.Command) {
	cmd.Flags().IntP("layer-count", "c", 0, "The number of layers that can be compressed")
	cmd.Flags().StringP("layer-digest", "d", "", "The digest of the layer to be compressed")
	cmd.Flags().StringP("author", "a", "", `Author (e.g., "nerdctl contributor <nerdctl-dev@example.com>")`)
	cmd.Flags().StringP("message", "m", "", "Commit message")
}

func NewSquashCommand() *cobra.Command {
	var squashCommand = &cobra.Command{
		Use:           "squash [flags] SOURCE_IMAGE TAG_IMAGE",
		Short:         "Compress the number of layers of the image",
		Args:          helpers.IsExactArgs(2),
		RunE:          squashAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	addSquashFlags(squashCommand)
	return squashCommand
}

func processSquashCommandFlags(cmd *cobra.Command, args []string) (options types.ImageSquashOptions, err error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return options, err
	}
	layerCount, err := cmd.Flags().GetInt("layer-count")
	if err != nil {
		return options, err
	}
	layerDigest, err := cmd.Flags().GetString("layer-digest")
	if err != nil {
		return options, err
	}
	author, err := cmd.Flags().GetString("author")
	if err != nil {
		return options, err
	}
	message, err := cmd.Flags().GetString("message")
	if err != nil {
		return options, err
	}

	options = types.ImageSquashOptions{
		GOptions: globalOptions,

		Author:  author,
		Message: message,

		SourceImageRef:  args[0],
		TargetImageName: args[1],

		SquashLayerCount:  layerCount,
		SquashLayerDigest: layerDigest,
	}
	return options, nil
}

func squashAction(cmd *cobra.Command, args []string) error {
	options, err := processSquashCommandFlags(cmd, args)
	if err != nil {
		return err
	}
	if !options.GOptions.Experimental {
		return fmt.Errorf("squash is an experimental feature, please enable experimental mode")
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return image.Squash(ctx, client, options)
}
