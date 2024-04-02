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
	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/container"
	"github.com/spf13/cobra"
)

func newWaitCommand() *cobra.Command {
	var waitCommand = &cobra.Command{
		Use:               "wait [flags] CONTAINER [CONTAINER, ...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Block until one or more containers stop, then print their exit codes.",
		RunE:              containerWaitAction,
		ValidArgsFunction: waitShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	return waitCommand
}

func processContainerWaitOptions(cmd *cobra.Command) (types.ContainerWaitOptions, error) {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return types.ContainerWaitOptions{}, err
	}
	return types.ContainerWaitOptions{
		Stdout:   cmd.OutOrStdout(),
		GOptions: globalOptions,
	}, nil
}

func containerWaitAction(cmd *cobra.Command, args []string) error {
	options, err := processContainerWaitOptions(cmd)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return container.Wait(ctx, client, args, options)
}

func waitShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show running container names
	statusFilterFn := func(st containerd.ProcessStatus) bool {
		return st == containerd.Running
	}
	return shellCompleteContainerNames(cmd, statusFilterFn)
}
