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

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/container"
)

func RestartCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:               "restart [flags] CONTAINER [CONTAINER, ...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Restart one or more running containers",
		RunE:              restartAction,
		ValidArgsFunction: startShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	cmd.Flags().UintP("time", "t", 10, "Seconds to wait for stop before killing it")
	cmd.Flags().StringP("signal", "s", "", "Signal to send to stop the container, before killing it")
	return cmd
}

func restartOptions(cmd *cobra.Command) (types.ContainerRestartOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.ContainerRestartOptions{}, err
	}

	var timeout *time.Duration
	if cmd.Flags().Changed("time") {
		// Seconds to wait for stop before killing it
		timeValue, err := cmd.Flags().GetUint("time")
		if err != nil {
			return types.ContainerRestartOptions{}, err
		}
		t := time.Duration(timeValue) * time.Second
		timeout = &t
	}

	var signal string
	if cmd.Flags().Changed("signal") {
		// Signal to send to stop the container, before killing it
		sig, err := cmd.Flags().GetString("signal")
		if err != nil {
			return types.ContainerRestartOptions{}, err
		}
		signal = sig
	}

	return types.ContainerRestartOptions{
		Stdout:  cmd.OutOrStdout(),
		GOption: globalOptions,
		Timeout: timeout,
		Signal:  signal,
	}, err
}

func restartAction(cmd *cobra.Command, args []string) error {
	options, err := restartOptions(cmd)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOption.Namespace, options.GOption.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return container.Restart(ctx, client, args, options)
}
