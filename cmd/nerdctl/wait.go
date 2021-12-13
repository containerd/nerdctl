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
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
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

func containerWaitAction(cmd *cobra.Command, args []string) error {
	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	var g errgroup.Group

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			waitContainer(ctx, cmd, client, found.Container.ID(), &g)
			return nil
		},
	}

	for _, req := range args {
		n, _ := walker.Walk(ctx, req)
		if n == 0 {
			return fmt.Errorf("no such container: %s", req)
		}
	}

	if err := g.Wait(); err != nil {
		return err
	}
	return nil
}

func waitContainer(ctx context.Context, cmd *cobra.Command, client *containerd.Client, id string, g *errgroup.Group) error {
	container, err := client.LoadContainer(ctx, id)
	if err != nil {
		return err
	}

	task, err := container.Task(ctx, cio.Load)
	if err != nil {
		return err
	}

	g.Go(func() error {
		statusC, err := task.Wait(ctx)
		if err != nil {
			return err
		}

		status := <-statusC
		code, _, err := status.Result()
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%d\n", int(code))
		return nil
	})

	return nil
}

func waitShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show running container names
	statusFilterFn := func(st containerd.ProcessStatus) bool {
		return st == containerd.Running
	}
	return shellCompleteContainerNames(cmd, statusFilterFn)
}
