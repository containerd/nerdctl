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
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
)

var waitCommand = &cli.Command{
	Name:         "wait",
	Usage:        "Block until one or more containers stop, then print their exit codes.",
	Action:       containerWaitAction,
	BashComplete: waitBashComplete,
}

func containerWaitAction(clicontext *cli.Context) error {
	if clicontext.NArg() == 0 {
		return errors.Errorf("requires at least 1 argument")
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	var g errgroup.Group

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			waitContainer(ctx, clicontext, client, found.Container.ID(), &g)
			return nil
		},
	}

	for _, req := range clicontext.Args().Slice() {
		n, _ := walker.Walk(ctx, req)
		if n == 0 {
			return errors.Errorf("no such container: %s", req)
		}
	}

	if err := g.Wait(); err != nil {
		return err
	}
	return nil
}

func waitContainer(ctx context.Context, clicontext *cli.Context, client *containerd.Client, id string, g *errgroup.Group) error {
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
		fmt.Fprintf(clicontext.App.Writer, "%d\n", int(code))
		return nil
	})

	return nil
}

func waitBashComplete(clicontext *cli.Context) {
	coco := parseCompletionContext(clicontext)
	if coco.boring || coco.flagTakesValue {
		defaultBashComplete(clicontext)
		return
	}
	// show running container names
	statusFilterFn := func(st containerd.ProcessStatus) bool {
		return st == containerd.Running
	}
	bashCompleteContainerNames(clicontext, statusFilterFn)
}
