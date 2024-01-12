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
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/container"
	"github.com/spf13/cobra"
)

func newStopCommand() *cobra.Command {
	var stopCommand = &cobra.Command{
		Use:               "stop [flags] CONTAINER [CONTAINER, ...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Stop one or more running containers",
		RunE:              stopAction,
		ValidArgsFunction: stopShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	stopCommand.Flags().IntP("time", "t", 10, "Seconds to wait before sending a SIGKILL")
	return stopCommand
}

func processContainerStopOptions(cmd *cobra.Command) (types.ContainerStopOptions, error) {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return types.ContainerStopOptions{}, err
	}
	var timeout *time.Duration
	if cmd.Flags().Changed("time") {
		timeValue, err := cmd.Flags().GetInt("time")
		if err != nil {
			return types.ContainerStopOptions{}, err
		}
		t := time.Duration(timeValue) * time.Second
		timeout = &t
	}
	return types.ContainerStopOptions{
		Stdout:   cmd.OutOrStdout(),
		Stderr:   cmd.ErrOrStderr(),
		GOptions: globalOptions,
		Timeout:  timeout,
	}, nil
}

func stopAction(cmd *cobra.Command, args []string) error {
	options, err := processContainerStopOptions(cmd)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return container.Stop(ctx, client, args, options)
}

func stopShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show non-stopped container names
	statusFilterFn := func(st containerd.ProcessStatus) bool {
		return st != containerd.Stopped && st != containerd.Created && st != containerd.Unknown
	}
	return shellCompleteContainerNames(cmd, statusFilterFn)
}
