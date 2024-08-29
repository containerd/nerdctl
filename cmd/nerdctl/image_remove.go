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

func newRmiCommand() *cobra.Command {
	var rmiCommand = &cobra.Command{
		Use:               "rmi [flags] IMAGE [IMAGE, ...]",
		Short:             "Remove one or more images",
		Args:              cobra.MinimumNArgs(1),
		RunE:              rmiAction,
		ValidArgsFunction: rmiShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	rmiCommand.Flags().BoolP("force", "f", false, "Force removal of the image")
	// Alias `-a` is reserved for `--all`. Should be compatible with `podman rmi --all`.
	rmiCommand.Flags().Bool("async", false, "Asynchronous mode")
	return rmiCommand
}

func processImageRemoveOptions(cmd *cobra.Command) (types.ImageRemoveOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.ImageRemoveOptions{}, err
	}

	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return types.ImageRemoveOptions{}, err
	}
	async, err := cmd.Flags().GetBool("async")
	if err != nil {
		return types.ImageRemoveOptions{}, err
	}

	return types.ImageRemoveOptions{
		Stdout:   cmd.OutOrStdout(),
		GOptions: globalOptions,
		Force:    force,
		Async:    async,
	}, nil
}

func rmiAction(cmd *cobra.Command, args []string) error {
	options, err := processImageRemoveOptions(cmd)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return image.Remove(ctx, client, args, options)
}

func rmiShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show image names
	return completion.ShellCompleteImageNames(cmd)
}
