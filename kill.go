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
	"os"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/errdefs"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

var killCommand = &cli.Command{
	Name:      "kill",
	Usage:     "Kill one or more running containers",
	ArgsUsage: "[flags] CONTAINER [CONTAINER, ...]",
	Action:    killAction,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "signal",
			Aliases: []string{"s"},
			Usage:   "Signal to send to the container",
			Value:   "KILL",
		},
	},
}

func killAction(clicontext *cli.Context) error {
	killSignal := clicontext.String("signal")

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
	var origID string
	var isError bool

	for _, id := range clicontext.Args().Slice() {
		// Save the original id passed from the CLI.
		origID = id
		if len(id) < idLength {
			found := 0
			for _, ctID := range allIDs {
				if strings.HasPrefix(ctID, id) {
					id = ctID
					found++
				}
			}
			if found == 0 || found > 1 {
				fmt.Fprintf(clicontext.App.Writer, "Error response from daemon: Cannot kill container: %s: No such container: %s\n", origID, origID)
				isError = true
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
				fmt.Fprintf(clicontext.App.Writer, "Error response from daemon: Cannot kill container %s: No such container: %s\n", origID, origID)
				isError = true
				continue
			}
			return err
		}

		if !strings.HasPrefix(killSignal, "SIG") {
			killSignal = "SIG" + killSignal
		}

		signal, err := containerd.ParseSignal(killSignal)
		if err != nil {
			return err
		}

		isRunning, err := isContainerRunning(task, ctx)
		if err != nil {
			return err
		}

		if !isRunning {
			// Task is not running anymore, no need to send signal.
			fmt.Fprintf(clicontext.App.Writer, "Error response from daemon: Cannot kill container %s: Container %s is not running\n", origID, origID)
			isError = true
			continue
		}

		if err = task.Kill(ctx, signal); err != nil {
			return err
		}
		fmt.Fprintf(clicontext.App.Writer, "%s\n", origID)
	}

	if isError {
		os.Exit(1)
	}

	return nil
}
