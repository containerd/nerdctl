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
	"context"
	"fmt"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"

	"github.com/spf13/cobra"
)

func newUnpauseCommand() *cobra.Command {
	var unpauseCommand = &cobra.Command{
		Use:               "unpause [flags] CONTAINER [CONTAINER, ...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Unpause all processes within one or more containers",
		RunE:              unpauseAction,
		ValidArgsFunction: unpauseShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	return unpauseCommand
}

func unpauseAction(cmd *cobra.Command, args []string) error {
	namespace, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	}
	address, err := cmd.Flags().GetString("address")
	if err != nil {
		return err
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), namespace, address)
	if err != nil {
		return err
	}
	defer cancel()

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			if err := unpauseContainer(ctx, client, found.Container.ID()); err != nil {
				return err
			}

			_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\n", found.Req)
			return err
		},
	}
	for _, req := range args {
		n, err := walker.Walk(ctx, req)
		if err != nil {
			return err
		} else if n == 0 {
			return fmt.Errorf("no such container %s", req)
		}
	}
	return nil
}

func unpauseContainer(ctx context.Context, client *containerd.Client, id string) error {
	container, err := client.LoadContainer(ctx, id)
	if err != nil {
		return err
	}

	task, err := container.Task(ctx, cio.Load)
	if err != nil {
		return err
	}

	status, err := task.Status(ctx)
	if err != nil {
		return err
	}

	switch status.Status {
	case containerd.Paused:
		return task.Resume(ctx)
	default:
		return fmt.Errorf("container %s is not paused", id)
	}
}

func unpauseShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show paused container names
	statusFilterFn := func(st containerd.ProcessStatus) bool {
		return st == containerd.Paused
	}
	return shellCompleteContainerNames(cmd, statusFilterFn)
}
