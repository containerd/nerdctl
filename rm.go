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
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/errdefs"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var rmCommand = &cli.Command{
	Name:      "rm",
	Usage:     "Remove one or more containers",
	ArgsUsage: "[flags] CONTAINER [CONTAINER, ...]",
	Action:    rmAction,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "force",
			Aliases: []string{"f"},
			Usage:   "Force the removal of a running|paused|unknown container (uses SIGKILL)",
		},
	},
}

func rmAction(clicontext *cli.Context) error {
	const idLength = 64

	if clicontext.NArg() == 0 {
		return errors.Errorf("requires at least 1 argument")
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	force := clicontext.Bool("force")

	containers, err := client.Containers(ctx)
	if err != nil {
		return err
	}
	allIDs := make([]string, len(containers))
	for i, c := range containers {
		allIDs[i] = c.ID()
	}
	for _, id := range clicontext.Args().Slice() {
		if len(id) < idLength {
			found := 0
			for _, ctID := range allIDs {
				if strings.HasPrefix(ctID, id) {
					id = ctID
					found++
				}
			}
			if found > 1 {
				logrus.Errorf("Ambiguous container ID: %s", id)
				continue
			}
		}
		if err := removeContainer(ctx, client, id, force); err != nil {
			return err
		}
		fmt.Println(id)
	}
	return nil
}

func removeContainer(ctx context.Context, client *containerd.Client, id string, force bool) error {
	container, err := client.LoadContainer(ctx, id)
	if err != nil {
		return err
	}

	task, err := container.Task(ctx, cio.Load)
	if err != nil {
		if errdefs.IsNotFound(err) {
			if container.Delete(ctx, containerd.WithSnapshotCleanup) != nil {
				return container.Delete(ctx)
			}
		}
		return err
	}

	status, err := task.Status(ctx)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return err
	}

	switch status.Status {
	case containerd.Created, containerd.Stopped:
		if _, err := task.Delete(ctx); err != nil && !errdefs.IsNotFound(err) {
			return errors.Wrapf(err, "failed to delete task %v", id)
		}
	default:
		if !force {
			return errors.Errorf("cannot remove a %v container %v", status.Status, id)
		}

		_, err := task.Delete(ctx, containerd.WithProcessKill)
		if err != nil && !errdefs.IsNotFound(err) {
			return errors.Wrapf(err, "failed to delete task %v", id)
		}
	}
	var delOpts []containerd.DeleteOpts
	if _, err := container.Image(ctx); err == nil {
		delOpts = append(delOpts, containerd.WithSnapshotCleanup)
	}
	return container.Delete(ctx, delOpts...)
}
