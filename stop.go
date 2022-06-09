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
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newStopCommand() *cobra.Command {
	var stopCommand = &cobra.Command{
		Use:               "stop [flags] CONTAINER [CONTAINER, ...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Stop one or more running containers",
		RunE:              stopAction,
		ValidArgsFunction: stopShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	stopCommand.Flags().StringP("time", "t", "10", "Seconds to wait for stop before killing it")
	return stopCommand
}

func stopAction(cmd *cobra.Command, args []string) error {
	// Time to wait after sending a SIGTERM and before sending a SIGKILL.
	// Default is 10 seconds.
	timeoutStr, err := cmd.Flags().GetString("time")
	if err != nil {
		return err
	}
	timeoutStr = timeoutStr + "s"

	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if err := stopContainer(ctx, found.Container, timeout); err != nil {
				if errdefs.IsNotFound(err) {
					fmt.Fprintf(cmd.ErrOrStderr(), "Error response from daemon: No such container: %s\n", found.Req)
					return nil
				}
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

func stopContainer(ctx context.Context, container containerd.Container, timeout time.Duration) error {
	if err := updateContainerStoppedLabel(ctx, container, true); err != nil {
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

	paused := false

	switch status.Status {
	case containerd.Created, containerd.Stopped:
		return nil
	case containerd.Paused, containerd.Pausing:
		paused = true
	default:
	}

	// NOTE: ctx is main context so that it's ok to use for task.Wait().
	exitCh, err := task.Wait(ctx)
	if err != nil {
		return err
	}

	if timeout > 0 {
		signal, err := containerd.ParseSignal("SIGTERM")
		if err != nil {
			return err
		}

		if err := task.Kill(ctx, signal); err != nil {
			return err
		}

		// signal will be sent once resume is finished
		if paused {
			if err := task.Resume(ctx); err != nil {
				logrus.Warnf("Cannot unpause container %s: %s", container.ID(), err)
			} else {
				// no need to do it again when send sigkill signal
				paused = false
			}
		}

		sigtermCtx, sigtermCtxCancel := context.WithTimeout(ctx, timeout)
		defer sigtermCtxCancel()

		err = waitContainerStop(sigtermCtx, exitCh, container.ID())
		if err == nil {
			return nil
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	signal, err := containerd.ParseSignal("SIGKILL")
	if err != nil {
		return err
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
	return waitContainerStop(ctx, exitCh, container.ID())
}

func waitContainerStop(ctx context.Context, exitCh <-chan containerd.ExitStatus, id string) error {
	select {
	case <-ctx.Done():
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("wait container %v: %w", id, err)
		}
		return nil
	case status := <-exitCh:
		return status.Error()
	}
}

func stopShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show non-stopped container names
	statusFilterFn := func(st containerd.ProcessStatus) bool {
		return st != containerd.Stopped && st != containerd.Created && st != containerd.Unknown
	}
	return shellCompleteContainerNames(cmd, statusFilterFn)
}
