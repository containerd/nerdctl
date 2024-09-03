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

func NewRenameCommand() *cobra.Command {
	var renameCommand = &cobra.Command{
		Use:               "rename [flags] CONTAINER NEW_NAME",
		Args:              helpers.IsExactArgs(2),
		Short:             "rename a container",
		RunE:              renameAction,
		ValidArgsFunction: renameShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	return renameCommand
}

func processContainerRenameOptions(cmd *cobra.Command) (types.ContainerRenameOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.ContainerRenameOptions{}, err
	}
	return types.ContainerRenameOptions{
		GOptions: globalOptions,
		Stdout:   cmd.OutOrStdout(),
	}, nil
}

func renameAction(cmd *cobra.Command, args []string) error {
	options, err := processContainerRenameOptions(cmd)
	if err != nil {
		return err
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()
	return container.Rename(ctx, client, args[0], args[1], options)
}
func renameShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completion.ContainerNames(cmd, nil)
}
