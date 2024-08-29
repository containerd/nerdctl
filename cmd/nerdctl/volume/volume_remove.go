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

package volume

import (
	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/volume"
)

func newVolumeRmCommand() *cobra.Command {
	volumeRmCommand := &cobra.Command{
		Use:               "rm [flags] VOLUME [VOLUME...]",
		Aliases:           []string{"remove"},
		Short:             "Remove one or more volumes",
		Long:              "NOTE: You cannot remove a volume that is in use by a container.",
		Args:              cobra.MinimumNArgs(1),
		RunE:              volumeRmAction,
		ValidArgsFunction: volumeRmShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	volumeRmCommand.Flags().BoolP("force", "f", false, "(unimplemented yet)")
	return volumeRmCommand
}

func processVolumeRmOptions(cmd *cobra.Command) (types.VolumeRemoveOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.VolumeRemoveOptions{}, err
	}
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return types.VolumeRemoveOptions{}, err
	}
	return types.VolumeRemoveOptions{
		GOptions: globalOptions,
		Force:    force,
		Stdout:   cmd.OutOrStdout(),
	}, nil
}

func volumeRmAction(cmd *cobra.Command, args []string) error {
	options, err := processVolumeRmOptions(cmd)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return volume.Remove(ctx, client, args, options)
}

func volumeRmShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show volume names
	return completion.ShellCompleteVolumeNames(cmd)
}
