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
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/cmd/container"

	"github.com/spf13/cobra"
)

func newPsCommand() *cobra.Command {
	var psCommand = &cobra.Command{
		Use:           "ps",
		Args:          cobra.NoArgs,
		Short:         "List containers",
		RunE:          psAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	psCommand.Flags().BoolP("all", "a", false, "Show all containers (default shows just running)")
	psCommand.Flags().IntP("last", "n", -1, "Show n last created containers (includes all states)")
	psCommand.Flags().BoolP("latest", "l", false, "Show the latest created container (includes all states)")
	psCommand.Flags().Bool("no-trunc", false, "Don't truncate output")
	psCommand.Flags().BoolP("quiet", "q", false, "Only display container IDs")
	psCommand.Flags().BoolP("size", "s", false, "Display total file sizes")

	// Alias "-f" is reserved for "--filter"
	psCommand.Flags().String("format", "", "Format the output using the given Go template, e.g, '{{json .}}', 'wide'")
	psCommand.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json", "table", "wide"}, cobra.ShellCompDirectiveNoFileComp
	})
	psCommand.Flags().StringSliceP("filter", "f", nil, "Filter matches containers based on given conditions")
	return psCommand
}

func processContainerListOptions(cmd *cobra.Command) (types.ContainerListOptions, error) {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return types.ContainerListOptions{}, err
	}
	all, err := cmd.Flags().GetBool("all")
	if err != nil {
		return types.ContainerListOptions{}, err
	}
	latest, err := cmd.Flags().GetBool("latest")
	if err != nil {
		return types.ContainerListOptions{}, err
	}

	lastN, err := cmd.Flags().GetInt("last")
	if err != nil {
		return types.ContainerListOptions{}, err
	}
	if lastN == -1 && latest {
		lastN = 1
	}

	filters, err := cmd.Flags().GetStringSlice("filter")
	if err != nil {
		return types.ContainerListOptions{}, err
	}

	noTrunc, err := cmd.Flags().GetBool("no-trunc")
	if err != nil {
		return types.ContainerListOptions{}, err
	}
	trunc := !noTrunc

	quiet, err := cmd.Flags().GetBool("quiet")
	if err != nil {
		return types.ContainerListOptions{}, err
	}
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return types.ContainerListOptions{}, err
	}

	size := false
	if !quiet {
		size, err = cmd.Flags().GetBool("size")
		if err != nil {
			return types.ContainerListOptions{}, err
		}
	}

	return types.ContainerListOptions{
		Stdout:   cmd.OutOrStdout(),
		GOptions: globalOptions,
		All:      all,
		LastN:    lastN,
		Truncate: trunc,
		Quiet:    quiet,
		Size:     size,
		Format:   format,
		Filters:  filters,
	}, nil
}

func psAction(cmd *cobra.Command, args []string) error {
	options, err := processContainerListOptions(cmd)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return container.List(ctx, client, options)
}
