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
	"github.com/containerd/nerdctl/v2/pkg/cmd/volume"
)

func newVolumeInspectCommand() *cobra.Command {
	volumeInspectCommand := &cobra.Command{
		Use:               "inspect [flags] VOLUME [VOLUME...]",
		Short:             "Display detailed information on one or more volumes",
		Args:              cobra.MinimumNArgs(1),
		RunE:              volumeInspectAction,
		ValidArgsFunction: volumeInspectShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	volumeInspectCommand.Flags().StringP("format", "f", "", "Format the output using the given Go template, e.g, '{{json .}}'")
	volumeInspectCommand.Flags().BoolP("size", "s", false, "Display the disk usage of the volume")
	volumeInspectCommand.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json"}, cobra.ShellCompDirectiveNoFileComp
	})
	return volumeInspectCommand
}

func processVolumeInspectOptions(cmd *cobra.Command) (types.VolumeInspectOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.VolumeInspectOptions{}, err
	}
	volumeSize, err := cmd.Flags().GetBool("size")
	if err != nil {
		return types.VolumeInspectOptions{}, err
	}
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return types.VolumeInspectOptions{}, err
	}
	return types.VolumeInspectOptions{
		GOptions: globalOptions,
		Format:   format,
		Size:     volumeSize,
		Stdout:   cmd.OutOrStdout(),
	}, nil
}

func volumeInspectAction(cmd *cobra.Command, args []string) error {
	options, err := processVolumeInspectOptions(cmd)
	if err != nil {
		return err
	}
	return volume.Inspect(cmd.Context(), args, options)
}

func volumeInspectShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show volume names
	return completion.ShellCompleteVolumeNames(cmd)
}
