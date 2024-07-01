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
	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/container"
	"github.com/containerd/nerdctl/v2/pkg/consoleutil"
	"github.com/spf13/cobra"
)

func newStartCommand() *cobra.Command {
	var startCommand = &cobra.Command{
		Use:               "start [flags] CONTAINER [CONTAINER, ...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Start one or more running containers",
		RunE:              startAction,
		ValidArgsFunction: startShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}

	startCommand.Flags().SetInterspersed(false)
	startCommand.Flags().BoolP("attach", "a", false, "Attach STDOUT/STDERR and forward signals")
	startCommand.Flags().String("detach-keys", consoleutil.DefaultDetachKeys, "Override the default detach keys")
	startCommand.Flags().BoolP("interactive", "i", false, "Keep STDIN open even if not attached")

	return startCommand
}

func processContainerStartOptions(cmd *cobra.Command) (types.ContainerStartOptions, error) {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return types.ContainerStartOptions{}, err
	}
	attach, err := cmd.Flags().GetBool("attach")
	if err != nil {
		return types.ContainerStartOptions{}, err
	}
	detachKeys, err := cmd.Flags().GetString("detach-keys")
	if err != nil {
		return types.ContainerStartOptions{}, err
	}
	interactive, err := cmd.Flags().GetBool("interactive")
	if err != nil {
		return types.ContainerStartOptions{}, err
	}
	return types.ContainerStartOptions{
		Stdout:      cmd.OutOrStdout(),
		GOptions:    globalOptions,
		Attach:      attach,
		DetachKeys:  detachKeys,
		Interactive: interactive,
	}, nil
}

func startAction(cmd *cobra.Command, args []string) error {
	options, err := processContainerStartOptions(cmd)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return container.Start(ctx, client, args, options)
}

func startShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show non-running container names
	statusFilterFn := func(st containerd.ProcessStatus) bool {
		return st != containerd.Running && st != containerd.Unknown
	}
	return shellCompleteContainerNames(cmd, statusFilterFn)
}
