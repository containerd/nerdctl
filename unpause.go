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

	"github.com/AkihiroSuda/nerdctl/pkg/idutil"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

var unpauseCommand = &cli.Command{
	Name:      "unpause",
	Usage:     "Unpause all processes within one or more containers",
	ArgsUsage: "[flags] CONTAINER [CONTAINER, ...]",
	Action:    unpauseAction,
}

func unpauseAction(clicontext *cli.Context) error {
	if clicontext.NArg() == 0 {
		return errors.Errorf("requires at least 1 argument")
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	argIDs := clicontext.Args().Slice()

	return idutil.WalkContainers(ctx, client, argIDs, func(ctx context.Context, client *containerd.Client, shortID, ID string) error {
		if err := unpauseContainer(ctx, client, ID); err != nil {
			return err
		}

		_, err := fmt.Fprintf(clicontext.App.Writer, "%s\n", shortID)
		return err
	})
}

func unpauseContainer(ctx context.Context, client *containerd.Client, id string) error {
	container, err := client.LoadContainer(ctx, id)
	if err != nil {
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

	switch status.Status {
	case containerd.Paused:
		return task.Resume(ctx)
	default:
		return errors.Errorf("Container %s is not paused", id)
	}
}
