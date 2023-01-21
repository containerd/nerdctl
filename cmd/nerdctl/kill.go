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
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/cmd/container"
	"github.com/spf13/cobra"
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
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	killSignal, err := cmd.Flags().GetString("signal")
	if err != nil {
		return err
	}
	return container.Kill(cmd.Context(), args, types.KillOptions{
		GOptions:   globalOptions,
		KillSignal: killSignal,
		Stdout:     cmd.OutOrStdout(),
		Stderr:     cmd.ErrOrStderr(),
	})
}

func killShellComplete(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	// show non-stopped container names
	statusFilterFn := func(st containerd.ProcessStatus) bool {
		return st != containerd.Stopped && st != containerd.Created && st != containerd.Unknown
	}
	return shellCompleteContainerNames(cmd, statusFilterFn)
}
