/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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
	"fmt"
	"strings"
	"time"
	"context"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/errdefs"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

var stopCommand = &cli.Command{
	Name:      "stop",
	Usage:     "Stop one or more running containers",
	ArgsUsage: "[flags] CONTAINER [CONTAINER, ...]",
	Action:    stopAction,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "time",
			Aliases: []string{"t"},
			Usage:   "Seconds to wait for stop before killing it",
			Value:   "10",
		},
	},
}

func stopAction(clicontext *cli.Context) error {
	// Time to wait after sending a SIGTERM and before sending a SIGKILL.
	// Default is 10 seconds.
	timeoutStr := clicontext.String("time")
	timeoutStr = timeoutStr + "s"

	if clicontext.NArg() == 0 {
		return errors.Errorf("requires at least 1 argument")
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	containers, err := client.Containers(ctx)
	if err != nil {
		return err
	}

	allIDs := make([]string, len(containers))
	for i, c := range containers {
		allIDs[i] = c.ID()
	}

	const idLength = 64
	for _, id := range clicontext.Args().Slice() {
		if len(id) < idLength {
			found := 0
			for _, ctID := range allIDs {
				if strings.HasPrefix(ctID, id) {
					id = ctID
					found++
				}
			}
			if found == 0 || found > 1 {
				fmt.Fprintf(clicontext.App.Writer, "Error response from daemon: No such container: %s\n", id)
				continue
			}
		}

		container, err := client.LoadContainer(ctx, id)
		if err != nil {
			return err
		}

		task, err := container.Task(ctx, cio.Load)
		if err != nil {
			if errdefs.IsNotFound(err) {
				fmt.Fprintf(clicontext.App.Writer, "Error response from daemon: No such container: %s\n", id)
				continue
			}
			return err
		}

		signal, err := containerd.ParseSignal("SIGTERM")
		if err != nil {
			return err
		}

		if err = task.Kill(ctx, signal); err != nil {
			return err
		}

		isRunning, err := isContainerRunning(task, ctx)
		if err != nil {
			return err
		}

		if !isRunning {
			// Task is not running anymore, no need to SIGKILL.
			fmt.Fprintf(clicontext.App.Writer, "%s\n", id[:12])
			continue
		}

		timeout, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return err
		}

		time.Sleep(timeout)

		signal, err = containerd.ParseSignal("SIGKILL")
		if err != nil {
			return err
		}

		isRunning, err = isContainerRunning(task, ctx)
		if err != nil {
			return err
		}

		if !isRunning {
			// Task is not running anymore, no need to SIGKILL.
			fmt.Fprintf(clicontext.App.Writer, "%s\n", id[:12])
			continue
		}

		if err = task.Kill(ctx, signal); err != nil {
			return err
		}
		fmt.Fprintf(clicontext.App.Writer, "%s\n", id[:12])
	}
	return nil
}

// Check if the container is running or not.
func isContainerRunning(task containerd.Task, ctx context.Context) (bool, error) {
	status, err := task.Status(ctx)
	if err != nil {
		return false, err
	}

	if status.Status != containerd.Running {
		return false, nil
	}
	return true, nil
}
