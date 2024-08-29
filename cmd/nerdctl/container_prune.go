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

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/container"
)

func newContainerPruneCommand() *cobra.Command {
	containerPruneCommand := &cobra.Command{
		Use:           "prune [flags]",
		Short:         "Remove all stopped containers",
		Args:          cobra.NoArgs,
		RunE:          containerPruneAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	containerPruneCommand.Flags().BoolP("force", "f", false, "Do not prompt for confirmation")
	return containerPruneCommand
}

func processContainerPruneOptions(cmd *cobra.Command) (types.ContainerPruneOptions, error) {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return types.ContainerPruneOptions{}, err
	}

	return types.ContainerPruneOptions{
		GOptions: globalOptions,
		Stdout:   cmd.OutOrStdout(),
	}, nil
}

func grantPrunePermission(cmd *cobra.Command) (bool, error) {
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return false, err
	}

	if !force {
		return helpers.Confirm(cmd, "WARNING! This will remove all stopped containers.")
	}
	return true, nil
}

func containerPruneAction(cmd *cobra.Command, _ []string) error {
	options, err := processContainerPruneOptions(cmd)
	if err != nil {
		return err
	}

	if ok, err := grantPrunePermission(cmd); err != nil {
		return err
	} else if !ok {
		return nil
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return container.Prune(ctx, client, options)
}
