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

package compose

import (
	"slices"

	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/compose"
	"github.com/containerd/nerdctl/v2/pkg/cmd/system"
	"github.com/containerd/nerdctl/v2/pkg/composer"
	"github.com/containerd/nerdctl/v2/pkg/labels"
)

func eventsCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:           "events [OPTIONS] [SERVICE...]",
		Short:         "Receive real time events from the containers of the project",
		Long:          "Receive real time events from the containers of the project, or only of the given services.\nNOTE: The output format is not compatible with Docker.",
		RunE:          eventsAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.Flags().String("format", "", "Format the output using the given Go template, e.g, '{{json .}}'")
	cmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().StringSliceP("filter", "f", []string{}, "Filter matches events based on given conditions")
	return cmd
}

func eventsAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	filters, err := cmd.Flags().GetStringSlice("filter")
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()
	options, err := getComposeOptions(cmd, globalOptions.DebugFull, globalOptions.Experimental)
	if err != nil {
		return err
	}
	c, err := compose.New(client, globalOptions, options, cmd.OutOrStdout(), cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	serviceNames, err := c.ServiceNames(args...)
	if err != nil {
		return err
	}
	project := c.ProjectName()

	// Like compose logs, release the compose lock before streaming. Events don't need it, and holding
	// it until the command exits would block every other compose command in the meantime.
	if err := composer.Unlock(); err != nil {
		return err
	}

	return system.Events(ctx, client, types.SystemEventsOptions{
		Stdout:   cmd.OutOrStdout(),
		GOptions: globalOptions,
		Format:   format,
		Filters:  filters,
		LabelsMatcher: func(containerLabels map[string]string) bool {
			return containerLabels[labels.ComposeProject] == project &&
				slices.Contains(serviceNames, containerLabels[labels.ComposeService])
		},
	})
}
