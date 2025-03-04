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
	"github.com/spf13/cobra"

	containerd "github.com/containerd/containerd/v2/client"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/container"
)

func PauseCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:               "pause [flags] CONTAINER [CONTAINER, ...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Pause all processes within one or more containers",
		RunE:              pauseAction,
		ValidArgsFunction: pauseShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	return cmd
}

func pauseOptions(cmd *cobra.Command) (types.ContainerPauseOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.ContainerPauseOptions{}, err
	}
	return types.ContainerPauseOptions{
		GOptions: globalOptions,
		Stdout:   cmd.OutOrStdout(),
	}, nil
}

func pauseAction(cmd *cobra.Command, args []string) error {
	options, err := pauseOptions(cmd)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return container.Pause(ctx, client, args, options)
}

func pauseShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show running container names
	statusFilterFn := func(st containerd.ProcessStatus) bool {
		return st == containerd.Running
	}
	return completion.ContainerNames(cmd, statusFilterFn)
}
