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
	"net/url"
	"syscall"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var startCommand = &cli.Command{
	Name:         "start",
	Usage:        "Start one or more running containers",
	ArgsUsage:    "[flags] CONTAINER [CONTAINER, ...]",
	Action:       startAction,
	BashComplete: startBashComplete,
}

func startAction(clicontext *cli.Context) error {
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
			if err := startContainer(ctx, found.Container); err != nil {
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

func startContainer(ctx context.Context, container containerd.Container) error {
	lab, err := container.Labels(ctx)
	if err != nil {
		return err
	}
	taskCIO := cio.NullIO
	if logURIStr := lab[labels.LogURI]; logURIStr != "" {
		logURI, err := url.Parse(logURIStr)
		if err != nil {
			return err
		}
		taskCIO = cio.LogURI(logURI)
	}
	if err = killContainer(ctx, container, syscall.SIGKILL); err != nil {
		logrus.WithError(err).Debug("failed to kill container (negligible in most case)")
	}
	if oldTask, err := container.Task(ctx, nil); err == nil {
		if _, err := oldTask.Delete(ctx); err != nil {
			logrus.WithError(err).Debug("failed to delete old task")
		}
	}
	task, err := container.NewTask(ctx, taskCIO)
	if err != nil {
		return err
	}
	return task.Start(ctx)
}

func startBashComplete(clicontext *cli.Context) {
	coco := parseCompletionContext(clicontext)
	if coco.boring || coco.flagTakesValue {
		defaultBashComplete(clicontext)
		return
	}
	// show container names (TODO: filter already running containers)
	bashCompleteContainerNames(clicontext)
}
