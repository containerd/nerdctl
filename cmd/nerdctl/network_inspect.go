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
	"github.com/containerd/nerdctl/v2/pkg/cmd/network"
)

func newNetworkInspectCommand() *cobra.Command {
	networkInspectCommand := &cobra.Command{
		Use:               "inspect [flags] NETWORK [NETWORK, ...]",
		Short:             "Display detailed information on one or more networks",
		Args:              cobra.MinimumNArgs(1),
		RunE:              networkInspectAction,
		ValidArgsFunction: networkInspectShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	networkInspectCommand.Flags().String("mode", "dockercompat", `Inspect mode, "dockercompat" for Docker-compatible output, "native" for containerd-native output`)
	networkInspectCommand.RegisterFlagCompletionFunc("mode", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"dockercompat", "native"}, cobra.ShellCompDirectiveNoFileComp
	})
	networkInspectCommand.Flags().StringP("format", "f", "", "Format the output using the given Go template, e.g, '{{json .}}'")
	networkInspectCommand.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json"}, cobra.ShellCompDirectiveNoFileComp
	})
	return networkInspectCommand
}

func networkInspectAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	mode, err := cmd.Flags().GetString("mode")
	if err != nil {
		return err
	}
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	return network.Inspect(cmd.Context(), types.NetworkInspectOptions{
		GOptions: globalOptions,
		Mode:     mode,
		Format:   format,
		Networks: args,
		Stdout:   cmd.OutOrStdout(),
	})
}

func networkInspectShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show network names, including "bridge"
	exclude := []string{"host", "none"}
	return completion.ShellCompleteNetworkNames(cmd, exclude)
}
