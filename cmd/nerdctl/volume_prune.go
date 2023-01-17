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
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/cmd/volume"
	"github.com/spf13/cobra"
)

func newVolumePruneCommand() *cobra.Command {
	volumePruneCommand := &cobra.Command{
		Use:           "prune [flags]",
		Short:         "Remove all unused local volumes",
		Args:          cobra.NoArgs,
		RunE:          volumePruneAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	volumePruneCommand.Flags().BoolP("force", "f", false, "Do not prompt for confirmation")
	return volumePruneCommand
}

func processVolumePruneCommandOptions(cmd *cobra.Command) (types.VolumePruneCommandOptions, error) {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return types.VolumePruneCommandOptions{}, err
	}
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return types.VolumePruneCommandOptions{}, err
	}
	return types.VolumePruneCommandOptions{
		GOptions: globalOptions,
		Force:    force,
	}, nil
}

func volumePruneAction(cmd *cobra.Command, _ []string) error {
	options, err := processVolumePruneCommandOptions(cmd)
	if err != nil {
		return err
	}
	return volume.Prune(cmd.Context(), options, cmd.InOrStdin(), cmd.OutOrStdout())
}
