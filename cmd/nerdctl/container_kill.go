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

	containerd "github.com/containerd/containerd/v2/client"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/container"
)

func newKillCommand() *cobra.Command {
	var killCommand = &cobra.Command{
		Use:               "kill [flags] CONTAINER [CONTAINER, ...]",
		Short:             "Kill one or more running containers",
		Args:              cobra.MinimumNArgs(1),
		RunE:              killAction,
		ValidArgsFunction: killShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	killCommand.Flags().StringP("signal", "s", "KILL", "Signal to send to the container")
	return killCommand
}

func killAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	killSignal, err := cmd.Flags().GetString("signal")
	if err != nil {
		return err
	}
	options := types.ContainerKillOptions{
		GOptions:   globalOptions,
		KillSignal: killSignal,
		Stdout:     cmd.OutOrStdout(),
		Stderr:     cmd.ErrOrStderr(),
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return container.Kill(ctx, client, args, options)
}

func killShellComplete(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	// show non-stopped container names
	statusFilterFn := func(st containerd.ProcessStatus) bool {
		return st != containerd.Stopped && st != containerd.Created && st != containerd.Unknown
	}
	return shellCompleteContainerNames(cmd, statusFilterFn)
}
