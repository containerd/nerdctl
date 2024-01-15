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
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/system"
	"github.com/spf13/cobra"
)

func newInfoCommand() *cobra.Command {
	var infoCommand = &cobra.Command{
		Use:           "info",
		Args:          cobra.NoArgs,
		Short:         "Display system-wide information",
		RunE:          infoAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	infoCommand.Flags().String("mode", "dockercompat", `Information mode, "dockercompat" for Docker-compatible output, "native" for containerd-native output`)
	infoCommand.RegisterFlagCompletionFunc("mode", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"dockercompat", "native"}, cobra.ShellCompDirectiveNoFileComp
	})
	infoCommand.Flags().StringP("format", "f", "", "Format the output using the given Go template, e.g, '{{json .}}'")
	infoCommand.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json"}, cobra.ShellCompDirectiveNoFileComp
	})
	return infoCommand
}

func processInfoOptions(cmd *cobra.Command) (types.SystemInfoOptions, error) {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return types.SystemInfoOptions{}, err
	}

	mode, err := cmd.Flags().GetString("mode")
	if err != nil {
		return types.SystemInfoOptions{}, err
	}
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return types.SystemInfoOptions{}, err
	}
	return types.SystemInfoOptions{
		GOptions: globalOptions,
		Mode:     mode,
		Format:   format,
		Stdout:   cmd.OutOrStdout(),
		Stderr:   cmd.OutOrStderr(),
	}, nil
}

func infoAction(cmd *cobra.Command, args []string) error {
	options, err := processInfoOptions(cmd)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return system.Info(ctx, client, options)
}
