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
	"os/signal"
	"strconv"
	"syscall"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/logging"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newLogsCommand() *cobra.Command {
	var logsCommand = &cobra.Command{
		Use:               "logs [flags] CONTAINER",
		Args:              cobra.ExactArgs(1),
		Short:             "Fetch the logs of a container. Currently, only containers created with `nerdctl run -d` are supported.",
		RunE:              logsAction,
		ValidArgsFunction: logsShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	logsCommand.Flags().BoolP("follow", "f", false, "Follow log output")
	logsCommand.Flags().BoolP("timestamps", "t", false, "Show timestamps")
	logsCommand.Flags().StringP("tail", "n", "all", "Number of lines to show from the end of the logs")
	logsCommand.Flags().String("since", "", "Show logs since timestamp (e.g. 2013-01-02T13:23:37Z) or relative (e.g. 42m for 42 minutes)")
	logsCommand.Flags().String("until", "", "Show logs before a timestamp (e.g. 2013-01-02T13:23:37Z) or relative (e.g. 42m for 42 minutes)")
	return logsCommand
}

func logsAction(cmd *cobra.Command, args []string) error {
	dataStore, err := getDataStore(cmd)
	if err != nil {
		return err
	}

	ns, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	}
	switch ns {
	case "moby", "k8s.io":
		logrus.Warn("Currently, `nerdctl logs` only supports containers created with `nerdctl run -d`")
	}

	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	follow, err := cmd.Flags().GetBool("follow")
	if err != nil {
		return err
	}
	tailArg, err := cmd.Flags().GetString("tail")
	if err != nil {
		return err
	}
	var tail uint
	if tailArg != "" {
		tail, err = getTailArgAsUint(tailArg)
		if err != nil {
			return err
		}
	}
	timestamps, err := cmd.Flags().GetBool("timestamps")
	if err != nil {
		return err
	}
	since, err := cmd.Flags().GetString("since")
	if err != nil {
		return err
	}
	until, err := cmd.Flags().GetString("until")
	if err != nil {
		return err
	}

	stopChannel := make(chan os.Signal, 1)
	// catch OS signals:
	signal.Notify(stopChannel, syscall.SIGTERM, syscall.SIGINT)

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			l, err := found.Container.Labels(ctx)
			if err != nil {
				return err
			}
			task, err := found.Container.Task(ctx, nil)
			if err != nil {
				return err
			}
			status, err := task.Status(ctx)
			if err != nil {
				return err
			}
			if status.Status != containerd.Running {
				follow = false
			}

			if follow {
				waitCh, err := task.Wait(ctx)
				if err != nil {
					return fmt.Errorf("failed to get wait channel for task %#v: %s", task, err)
				}

				// Setup goroutine to send stop event if container task finishes:
				go func() {
					<-waitCh
					logrus.Debugf("container task has finished, sending kill signal to log viewer")
					stopChannel <- os.Interrupt
				}()
			}

			logViewOpts := logging.LogViewOptions{
				ContainerID:       found.Container.ID(),
				Namespace:         l[labels.Namespace],
				DatastoreRootPath: dataStore,
				Follow:            follow,
				Timestamps:        timestamps,
				Tail:              tail,
				Since:             since,
				Until:             until,
			}
			logViewer, err := logging.InitContainerLogViewer(logViewOpts, stopChannel)
			if err != nil {
				return err
			}

			return logViewer.PrintLogsTo(os.Stdout, os.Stderr)
		},
	}
	req := args[0]
	n, err := walker.Walk(ctx, req)
	if err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("no such container %s", req)
	}
	return nil
}

func logsShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show container names (TODO: only show containers with logs)
	return shellCompleteContainerNames(cmd, nil)
}

// Attempts to parse the argument given to `-n/--tail` as a uint.
func getTailArgAsUint(arg string) (uint, error) {
	if arg == "all" {
		return 0, nil
	}
	num, err := strconv.Atoi(arg)
	if err != nil {
		return 0, fmt.Errorf("failed to parse `-n/--tail` argument %q: %s", arg, err)
	}
	if num < 0 {
		return 0, fmt.Errorf("`-n/--tail` argument must be positive, got: %d", num)
	}
	return uint(num), nil
}
