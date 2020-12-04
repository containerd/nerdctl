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
	"text/tabwriter"

	"github.com/containerd/containerd"
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
	w := tabwriter.NewWriter(os.Stdout, 4, 8, 4, ' ', 0)
	fmt.Fprintln(w, "CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
	for _, c := range containers {
		info, err := c.Info(ctx, containerd.WithoutRefreshedMetadata)
		if err != nil {
			return err
		}
		imageName := info.Image
		id := c.ID()
		if !clicontext.Bool("no-trunc") && len(id) > 12 {
			id = id[:12]
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			id,
			imageName,
			"", // COMMAND
			"", // CREATED
			"", // STATUS
			"", // PORTS,
			"", // NAMES
		); err != nil {
			return err
		}
	}
	return w.Flush()
}
