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
	"github.com/containerd/nerdctl/v2/pkg/consoleutil"
	"github.com/spf13/cobra"
)

func newAttachCommand() *cobra.Command {
	const shortHelp = "Attach stdin, stdout, and stderr to a running container."
	const longHelp = `Attach stdin, stdout, and stderr to a running container. For example:

1. 'nerdctl run -it --name test busybox' to start a container with a pty
2. 'ctrl-p ctrl-q' to detach from the container
3. 'nerdctl attach test' to attach to the container

Caveats:

- Currently only one attach session is allowed. When the second session tries to attach, currently no error will be returned from nerdctl.
  However, since behind the scenes, there's only one FIFO for stdin, stdout, and stderr respectively,
  if there are multiple sessions, all the sessions will be reading from and writing to the same 3 FIFOs, which will result in mixed input and partial output.
- Until dual logging (issue #1946) is implemented,
  a container that is spun up by either 'nerdctl run -d' or 'nerdctl start' (without '--attach') cannot be attached to.`

	var attachCommand = &cobra.Command{
		Use:               "attach [flags] CONTAINER",
		Args:              cobra.ExactArgs(1),
		Short:             shortHelp,
		Long:              longHelp,
		RunE:              containerAttachAction,
		ValidArgsFunction: attachShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	attachCommand.Flags().String("detach-keys", consoleutil.DefaultDetachKeys, "Override the default detach keys")
	return attachCommand
}

func processContainerAttachOptions(cmd *cobra.Command) (types.ContainerAttachOptions, error) {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return types.ContainerAttachOptions{}, err
	}
	detachKeys, err := cmd.Flags().GetString("detach-keys")
	if err != nil {
		return types.ContainerAttachOptions{}, err
	}
	return types.ContainerAttachOptions{
		GOptions:   globalOptions,
		Stdin:      cmd.InOrStdin(),
		Stdout:     cmd.OutOrStdout(),
		Stderr:     cmd.ErrOrStderr(),
		DetachKeys: detachKeys,
	}, nil
}

func containerAttachAction(cmd *cobra.Command, args []string) error {
	options, err := processContainerAttachOptions(cmd)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return container.Attach(ctx, client, args[0], options)
}

func attachShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	statusFilterFn := func(st containerd.ProcessStatus) bool {
		return st == containerd.Running
	}
	return shellCompleteContainerNames(cmd, statusFilterFn)
}
