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
	"github.com/containerd/nerdctl/v2/pkg/cmd/container"
)

func newCommitCommand() *cobra.Command {
	var commitCommand = &cobra.Command{
		Use:               "commit [flags] CONTAINER REPOSITORY[:TAG]",
		Short:             "Create a new image from a container's changes",
		Args:              IsExactArgs(2),
		RunE:              commitAction,
		ValidArgsFunction: commitShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	commitCommand.Flags().StringP("author", "a", "", `Author (e.g., "nerdctl contributor <nerdctl-dev@example.com>")`)
	commitCommand.Flags().StringP("message", "m", "", "Commit message")
	commitCommand.Flags().StringArrayP("change", "c", nil, "Apply Dockerfile instruction to the created image (supported directives: [CMD, ENTRYPOINT])")
	commitCommand.Flags().BoolP("pause", "p", true, "Pause container during commit")
	return commitCommand
}

func processCommitCommandOptions(cmd *cobra.Command) (types.ContainerCommitOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.ContainerCommitOptions{}, err
	}
	author, err := cmd.Flags().GetString("author")
	if err != nil {
		return types.ContainerCommitOptions{}, err
	}
	message, err := cmd.Flags().GetString("message")
	if err != nil {
		return types.ContainerCommitOptions{}, err
	}
	pause, err := cmd.Flags().GetBool("pause")
	if err != nil {
		return types.ContainerCommitOptions{}, err
	}

	change, err := cmd.Flags().GetStringArray("change")
	if err != nil {
		return types.ContainerCommitOptions{}, err
	}

	return types.ContainerCommitOptions{
		Stdout:   cmd.OutOrStdout(),
		GOptions: globalOptions,
		Author:   author,
		Message:  message,
		Pause:    pause,
		Change:   change,
	}, nil

}

func commitAction(cmd *cobra.Command, args []string) error {
	options, err := processCommitCommandOptions(cmd)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return container.Commit(ctx, client, args[1], args[0], options)
}

func commitShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return completion.ContainerNames(cmd, nil)
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}
