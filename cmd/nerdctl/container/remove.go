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

package container

import (
	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/container"
)

func RemoveCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:               "rm [flags] CONTAINER [CONTAINER, ...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Remove one or more containers",
		RunE:              removeAction,
		ValidArgsFunction: rmShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	cmd.Aliases = []string{"remove"}
	cmd.Flags().BoolP("force", "f", false, "Force the removal of a running|paused|unknown container (uses SIGKILL)")
	cmd.Flags().BoolP("volumes", "v", false, "Remove volumes associated with the container")
	return cmd
}

func removeAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}
	removeAnonVolumes, err := cmd.Flags().GetBool("volumes")
	if err != nil {
		return err
	}
	options := types.ContainerRemoveOptions{
		GOptions: globalOptions,
		Force:    force,
		Volumes:  removeAnonVolumes,
		Stdout:   cmd.OutOrStdout(),
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return container.Remove(ctx, client, args, options)
}

func rmShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show container names
	return completion.ContainerNames(cmd, nil)
}
