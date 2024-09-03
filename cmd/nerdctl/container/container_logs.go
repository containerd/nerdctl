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
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/container"
)

func NewLogsCommand() *cobra.Command {
	const shortUsage = "Fetch the logs of a container. Expected to be used with 'nerdctl run -d'."
	const longUsage = `Fetch the logs of a container.

The following containers are supported:
- Containers created with 'nerdctl run -d'. The log is currently empty for containers created without '-d'.
- Containers created with 'nerdctl compose'.
- Containers created with Kubernetes (EXPERIMENTAL).
`
	var logsCommand = &cobra.Command{
		Use:               "logs [flags] CONTAINER",
		Args:              helpers.IsExactArgs(1),
		Short:             shortUsage,
		Long:              longUsage,
		RunE:              logsAction,
		ValidArgsFunction: logsShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	logsCommand.Flags().BoolP("follow", "f", false, "Follow log output")
	logsCommand.Flags().BoolP("timestamps", "t", false, "Show timestamps")
	logsCommand.Flags().StringP("tail", "n", "all", "Number of lines to show from the end of the logs")
	logsCommand.Flags().String("since", "", "Show logs since timestamp (e.g. 2013-01-02T13:23:37Z) or relative (e.g. 42m for 42 minutes)")
	logsCommand.Flags().String("until", "", "Show logs before a timestamp (e.g. 2013-01-02T13:23:37Z) or relative (e.g. 42m for 42 minutes)")
	return logsCommand
}

func processContainerLogsOptions(cmd *cobra.Command) (types.ContainerLogsOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.ContainerLogsOptions{}, err
	}
	follow, err := cmd.Flags().GetBool("follow")
	if err != nil {
		return types.ContainerLogsOptions{}, err
	}
	tailArg, err := cmd.Flags().GetString("tail")
	if err != nil {
		return types.ContainerLogsOptions{}, err
	}
	var tail uint
	if tailArg != "" {
		tail, err = getTailArgAsUint(tailArg)
		if err != nil {
			return types.ContainerLogsOptions{}, err
		}
	}
	timestamps, err := cmd.Flags().GetBool("timestamps")
	if err != nil {
		return types.ContainerLogsOptions{}, err
	}
	since, err := cmd.Flags().GetString("since")
	if err != nil {
		return types.ContainerLogsOptions{}, err
	}
	until, err := cmd.Flags().GetString("until")
	if err != nil {
		return types.ContainerLogsOptions{}, err
	}
	return types.ContainerLogsOptions{
		Stdout:     cmd.OutOrStdout(),
		Stderr:     cmd.OutOrStderr(),
		GOptions:   globalOptions,
		Follow:     follow,
		Timestamps: timestamps,
		Tail:       tail,
		Since:      since,
		Until:      until,
	}, nil
}

func logsAction(cmd *cobra.Command, args []string) error {
	options, err := processContainerLogsOptions(cmd)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return container.Logs(ctx, client, args[0], options)
}

func logsShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show container names (TODO: only show containers with logs)
	return completion.ContainerNames(cmd, nil)
}

// Attempts to parse the argument given to `-n/--tail` as a uint.
func getTailArgAsUint(arg string) (uint, error) {
	if arg == "all" {
		return 0, nil
	}
	num, err := strconv.Atoi(arg)
	if err != nil {
		return 0, fmt.Errorf("failed to parse `-n/--tail` argument %q: %s", arg, err)
	}
	if num < 0 {
		return 0, fmt.Errorf("`-n/--tail` argument must be positive, got: %d", num)
	}
	return uint(num), nil
}
