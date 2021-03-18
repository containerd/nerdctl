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
	"context"
	"fmt"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var stopCommand = &cli.Command{
	Name:         "stop",
	Usage:        "Stop one or more running containers",
	ArgsUsage:    "[flags] CONTAINER [CONTAINER, ...]",
	Action:       stopAction,
	BashComplete: stopBashComplete,
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

	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if err := stopContainer(ctx, found.Container, timeout); err != nil {
				if errdefs.IsNotFound(err) {
					fmt.Fprintf(clicontext.App.ErrWriter, "Error response from daemon: No such container: %s\n", found.Req)
					return nil
				}
				return err
			}
			_, err := fmt.Fprintf(clicontext.App.Writer, "%s\n", found.Req)
			return err
		},
	}
	for _, req := range clicontext.Args().Slice() {
		n, err := walker.Walk(ctx, req)
		if err != nil {
			return err
		} else if n == 0 {
			return errors.Errorf("no such container %s", req)
		}
	}
	return nil
}

func stopContainer(ctx context.Context, container containerd.Container, timeout time.Duration) error {
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
		return errors.Wrapf(ctx.Err(), "wait container %v", id)
	case status := <-exitCh:
		return status.Error()
	}
}

func stopBashComplete(clicontext *cli.Context) {
	coco := parseCompletionContext(clicontext)
	if coco.boring || coco.flagTakesValue {
		defaultBashComplete(clicontext)
		return
	}
	// show container names (TODO: filter already stopped containers)
	bashCompleteContainerNames(clicontext)
}
