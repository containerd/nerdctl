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
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var killCommand = &cli.Command{
	Name:         "kill",
	Usage:        "Kill one or more running containers",
	ArgsUsage:    "[flags] CONTAINER [CONTAINER, ...]",
	Action:       killAction,
	BashComplete: killBashComplete,
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
	if !strings.HasPrefix(killSignal, "SIG") {
		killSignal = "SIG" + killSignal
	}

	signal, err := containerd.ParseSignal(killSignal)
	if err != nil {
		return err
	}

	if clicontext.NArg() == 0 {
		return errors.Errorf("requires at least 1 argument")
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if err := killContainer(ctx, found.Container, signal); err != nil {
				if errdefs.IsNotFound(err) {
					fmt.Fprintf(clicontext.App.ErrWriter, "Error response from daemon: Cannot kill container: %s: No such container: %s\n", found.Req, found.Req)
					os.Exit(1)
				}
				return err
			}
			_, err := fmt.Fprintf(clicontext.App.Writer, "%s\n", found.Container.ID())
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
		return errors.Errorf("cannot kill container: %s: Container %s is not running\n", container.ID(), container.ID())
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

func killBashComplete(clicontext *cli.Context) {
	coco := parseCompletionContext(clicontext)
	if coco.boring || coco.flagTakesValue {
		defaultBashComplete(clicontext)
		return
	}
	// show container names (TODO: filter already stopped containers)
	bashCompleteContainerNames(clicontext)
}
