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
	"time"

	"github.com/spf13/cobra"

	containerd "github.com/containerd/containerd/v2/client"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/container"
)

func StopCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:               "stop [flags] CONTAINER [CONTAINER, ...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Stop one or more running containers",
		RunE:              stopAction,
		ValidArgsFunction: stopShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	cmd.Flags().IntP("time", "t", 10, "Seconds to wait before sending a SIGKILL")
	cmd.Flags().StringP("signal", "s", "SIGTERM", "Signal to send to the container")
	return cmd
}

func stopOptions(cmd *cobra.Command) (types.ContainerStopOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
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
	var signal string
	if cmd.Flags().Changed("signal") {
		signalValue, err := cmd.Flags().GetString("signal")
		if err != nil {
			return types.ContainerStopOptions{}, err
		}
		signal = signalValue
	}
	return types.ContainerStopOptions{
		Stdout:   cmd.OutOrStdout(),
		Stderr:   cmd.ErrOrStderr(),
		GOptions: globalOptions,
		Timeout:  timeout,
		Signal:   signal,
	}, nil
}

func stopAction(cmd *cobra.Command, args []string) error {
	options, err := stopOptions(cmd)
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
	return completion.ContainerNames(cmd, statusFilterFn)
}
