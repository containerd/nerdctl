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
	"fmt"
	"strconv"

	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/cmd/container"
	"github.com/spf13/cobra"
)

func newLogsCommand() *cobra.Command {
	var logsCommand = &cobra.Command{
		Use:               "logs [flags] CONTAINER",
		Args:              IsExactArgs(1),
		Short:             "Fetch the logs of a container. Currently, only containers created with `nerdctl run -d` are supported.",
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
	globalOptions, err := processRootCmdFlags(cmd)
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
	return shellCompleteContainerNames(cmd, nil)
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
