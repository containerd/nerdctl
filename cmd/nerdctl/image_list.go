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

	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/image"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
)

func newImagesCommand() *cobra.Command {
	shortHelp := "List images"
	longHelp := shortHelp + `

Properties:
- REPOSITORY: Repository
- TAG:        Tag
- NAME:       Name of the image, --names for skip parsing as repository and tag.
- IMAGE ID:   OCI Digest. Usually different from Docker image ID. Shared for multi-platform images.
- CREATED:    Created time
- PLATFORM:   Platform
- SIZE:       Size of the unpacked snapshots
- BLOB SIZE:  Size of the blobs (such as layer tarballs) in the content store
`
	var imagesCommand = &cobra.Command{
		Use:                   "images [flags] [REPOSITORY[:TAG]]",
		Short:                 shortHelp,
		Long:                  longHelp,
		Args:                  cobra.MaximumNArgs(1),
		RunE:                  imagesAction,
		ValidArgsFunction:     imagesShellComplete,
		SilenceUsage:          true,
		SilenceErrors:         true,
		DisableFlagsInUseLine: true,
	}

	imagesCommand.Flags().BoolP("quiet", "q", false, "Only show numeric IDs")
	imagesCommand.Flags().Bool("no-trunc", false, "Don't truncate output")
	// Alias "-f" is reserved for "--filter"
	imagesCommand.Flags().String("format", "", "Format the output using the given Go template, e.g, '{{json .}}', 'wide'")
	imagesCommand.Flags().StringSliceP("filter", "f", []string{}, "Filter output based on conditions provided")
	imagesCommand.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json", "table", "wide"}, cobra.ShellCompDirectiveNoFileComp
	})
	imagesCommand.Flags().Bool("digests", false, "Show digests (compatible with Docker, unlike ID)")
	imagesCommand.Flags().Bool("names", false, "Show image names")
	imagesCommand.Flags().BoolP("all", "a", true, "(unimplemented yet, always true)")

	return imagesCommand
}

func processImageListOptions(cmd *cobra.Command, args []string) (types.ImageListOptions, error) {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return types.ImageListOptions{}, err
	}
	var filters []string

	if len(args) > 0 {
		canonicalRef, err := referenceutil.ParseAny(args[0])
		if err != nil {
			return types.ImageListOptions{}, err
		}
		filters = append(filters, fmt.Sprintf("name==%s", canonicalRef.String()))
		filters = append(filters, fmt.Sprintf("name==%s", args[0]))
	}
	quiet, err := cmd.Flags().GetBool("quiet")
	if err != nil {
		return types.ImageListOptions{}, err
	}
	noTrunc, err := cmd.Flags().GetBool("no-trunc")
	if err != nil {
		return types.ImageListOptions{}, err
	}
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return types.ImageListOptions{}, err
	}
	var inputFilters []string
	if cmd.Flags().Changed("filter") {
		inputFilters, err = cmd.Flags().GetStringSlice("filter")
		if err != nil {
			return types.ImageListOptions{}, err
		}
	}
	digests, err := cmd.Flags().GetBool("digests")
	if err != nil {
		return types.ImageListOptions{}, err
	}
	names, err := cmd.Flags().GetBool("names")
	if err != nil {
		return types.ImageListOptions{}, err
	}
	return types.ImageListOptions{
		GOptions:         globalOptions,
		Quiet:            quiet,
		NoTrunc:          noTrunc,
		Format:           format,
		Filters:          inputFilters,
		NameAndRefFilter: filters,
		Digests:          digests,
		Names:            names,
		All:              true,
		Stdout:           cmd.OutOrStdout(),
	}, nil

}

func imagesAction(cmd *cobra.Command, args []string) error {
	options, err := processImageListOptions(cmd, args)
	if err != nil {
		return err
	}
	if !options.All {
		options.All = true
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return image.ListCommandHandler(ctx, client, options)
}

func imagesShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		// show image names
		return shellCompleteImageNames(cmd)
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}
