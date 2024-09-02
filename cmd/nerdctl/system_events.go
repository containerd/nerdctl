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
	"github.com/containerd/nerdctl/v2/pkg/cmd/system"
)

func newEventsCommand() *cobra.Command {
	shortHelp := `Get real time events from the server`
	longHelp := shortHelp + "\nNOTE: The output format is not compatible with Docker."
	var eventsCommand = &cobra.Command{
		Use:           "events",
		Args:          cobra.NoArgs,
		Short:         shortHelp,
		Long:          longHelp,
		RunE:          eventsAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	eventsCommand.Flags().String("format", "", "Format the output using the given Go template, e.g, '{{json .}}'")
	eventsCommand.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json"}, cobra.ShellCompDirectiveNoFileComp
	})
	eventsCommand.Flags().StringSliceP("filter", "f", []string{}, "Filter matches containers based on given conditions")
	return eventsCommand
}

func processSystemEventsOptions(cmd *cobra.Command) (types.SystemEventsOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.SystemEventsOptions{}, err
	}
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return types.SystemEventsOptions{}, err
	}
	filters, err := cmd.Flags().GetStringSlice("filter")
	if err != nil {
		return types.SystemEventsOptions{}, err
	}
	return types.SystemEventsOptions{
		Stdout:   cmd.OutOrStdout(),
		GOptions: globalOptions,
		Format:   format,
		Filters:  filters,
	}, nil
}

func eventsAction(cmd *cobra.Command, args []string) error {
	options, err := processSystemEventsOptions(cmd)
	if err != nil {
		return err
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()
	return system.Events(ctx, client, options)
}
