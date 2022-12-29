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
	"os"
	"strings"
	"syscall"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"

	"github.com/moby/sys/signal"
	"github.com/sirupsen/logrus"
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
	killSignal, err := cmd.Flags().GetString("signal")
	if err != nil {
		return err
	}
	if !strings.HasPrefix(killSignal, "SIG") {
		killSignal = "SIG" + killSignal
	}

	signal, err := signal.ParseSignal(killSignal)
	if err != nil {
		return err
	}
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
			if err := killContainer(ctx, found.Container, signal); err != nil {
				if errdefs.IsNotFound(err) {
					fmt.Fprintf(cmd.ErrOrStderr(), "No such container: %s\n", found.Req)
					os.Exit(1)
				}
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\n", found.Container.ID())
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

func killContainer(ctx context.Context, container containerd.Container, signal syscall.Signal) error {
	task, err := container.Task(ctx, cio.Load)
	if err != nil {
		return err
	}

	status, err := task.Status(ctx)
	if err != nil {
		return err
	}

	paused := false

	switch status.Status {
	case containerd.Created, containerd.Stopped:
		return fmt.Errorf("cannot kill container: %s: Container %s is not running", container.ID(), container.ID())
	case containerd.Paused, containerd.Pausing:
		paused = true
	default:
	}

	if err := task.Kill(ctx, signal); err != nil {
		return err
	}

	// signal will be sent once resume is finished
	if paused {
		if err := task.Resume(ctx); err != nil {
			logrus.Warnf("Cannot unpause container %s: %s", container.ID(), err)
		}
	}
	return nil
}

func killShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show non-stopped container names
	statusFilterFn := func(st containerd.ProcessStatus) bool {
		return st != containerd.Stopped && st != containerd.Created && st != containerd.Unknown
	}
	return shellCompleteContainerNames(cmd, statusFilterFn)

}
