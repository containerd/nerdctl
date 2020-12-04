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
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/oci"
	"github.com/docker/go-units"
	"github.com/urfave/cli/v2"
)

var psCommand = &cli.Command{
	Name:   "ps",
	Usage:  "List containers",
	Action: psAction,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "all",
			Aliases: []string{"a"},
			Usage:   "(Currently ignored and always assumed to be true)",
		},
		&cli.BoolFlag{
			Name:  "no-trunc",
			Usage: "Don't truncate output",
		},
	},
}

func psAction(clicontext *cli.Context) error {
	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()
	containers, err := client.Containers(ctx)
	if err != nil {
		return err
	}
	return printContainers(ctx, clicontext, containers)
}

func printContainers(ctx context.Context, clicontext *cli.Context, containers []containerd.Container) error {
	trunc := !clicontext.Bool("no-trunc")

	w := tabwriter.NewWriter(os.Stdout, 4, 8, 4, ' ', 0)
	fmt.Fprintln(w, "CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
	for _, c := range containers {
		info, err := c.Info(ctx, containerd.WithoutRefreshedMetadata)
		if err != nil {
			return err
		}

		spec, err := c.Spec(ctx)
		if err != nil {
			return err
		}

		imageName := info.Image
		id := c.ID()
		if trunc && len(id) > 12 {
			id = id[:12]
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			id,
			imageName,
			containerCommand(spec, trunc),
			timeSinceInHuman(info.CreatedAt),
			containerStatus(ctx, c),
			"", // PORTS,
			"", // NAMES
		); err != nil {
			return err
		}
	}
	return w.Flush()
}

func containerStatus(ctx context.Context, c containerd.Container) string {
	// Just in case, there is something wrong in server.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	task, err := c.Task(ctx, nil)
	if err != nil {
		// NOTE: NotFound doesn't mean that container hasn't started.
		// In docker/CRI-containerd plugin, the task will be deleted
		// when it exits. So, the status will be "created" for this
		// case.
		if errdefs.IsNotFound(err) {
			return strings.Title(string(containerd.Created))
		}
		return strings.Title(string(containerd.Unknown))
	}

	status, err := task.Status(ctx)
	if err != nil {
		return strings.Title(string(containerd.Unknown))
	}

	switch s := status.Status; s {
	case containerd.Stopped:
		return fmt.Sprintf("%s (%v) %s", strings.Title(string(s)), status.ExitStatus, timeSinceInHuman(status.ExitTime))
	default:
		return strings.Title(string(s))
	}
}

func containerCommand(spec *oci.Spec, trunc bool) string {
	if spec == nil || spec.Process == nil {
		return ""
	}

	command := spec.Process.CommandLine + strings.Join(spec.Process.Args, " ")
	if trunc {
		command = ellipsis(command, 20)
	}
	return strconv.Quote(command)
}

func timeSinceInHuman(since time.Time) string {
	return units.HumanDuration(time.Now().Sub(since)) + " ago"
}

func ellipsis(str string, maxDisplayWidth int) string {
	if maxDisplayWidth <= 0 {
		return ""
	}

	lenStr := len(str)
	if maxDisplayWidth == 1 {
		if lenStr <= maxDisplayWidth {
			return str
		}
		return string(str[0])
	}

	if lenStr <= maxDisplayWidth {
		return str
	}
	return str[:maxDisplayWidth-1] + "…"
}
