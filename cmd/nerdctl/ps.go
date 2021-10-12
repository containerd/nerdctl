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
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/tabwriter"
	"text/template"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/docker/cli/templates"
	"github.com/pkg/errors"
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
			Usage:   "Show all containers (default shows just running)",
		},
		&cli.BoolFlag{
			Name:  "no-trunc",
			Usage: "Don't truncate output",
		},
		&cli.BoolFlag{
			Name:    "quiet",
			Aliases: []string{"q"},
			Usage:   "Only display container IDs",
		},
		&cli.StringFlag{
			Name: "format",
			// Alias "-f" is reserved for "--filter"
			Usage: "Format the output using the given Go template, e.g, '{{json .}}'",
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

type containerPrintable struct {
	Command   string
	CreatedAt string
	ID        string
	Image     string
	Names     string
	Ports     string
	Status    string
	// TODO: "Labels", "LocalVolumes", "Mounts", "Networks", "RunningFor", "Size", "State"
}

func printContainers(ctx context.Context, clicontext *cli.Context, containers []containerd.Container) error {
	trunc := !clicontext.Bool("no-trunc")
	all := clicontext.Bool("all")
	quiet := clicontext.Bool("quiet")
	w := clicontext.App.Writer
	var tmpl *template.Template
	switch format := clicontext.String("format"); format {
	case "", "table":
		w = tabwriter.NewWriter(clicontext.App.Writer, 4, 8, 4, ' ', 0)
		if !quiet {
			fmt.Fprintln(w, "CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
		}
	case "raw":
		return errors.New("unsupported format: \"raw\"")
	default:
		if quiet {
			return errors.New("format and quiet must not be specified together")
		}
		var err error
		tmpl, err = templates.Parse(format)
		if err != nil {
			return err
		}
	}

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

		cStatus := formatter.ContainerStatus(ctx, c)
		if !strings.HasPrefix(cStatus, "Up") && !all {
			continue
		}

		p := containerPrintable{
			Command:   formatter.InspectContainerCommand(spec, trunc),
			CreatedAt: info.CreatedAt.Round(time.Second).Local().String(), // format like "2021-08-07 02:19:45 +0900 JST"
			ID:        id,
			Image:     imageName,
			Names:     info.Labels[labels.Name],
			Ports:     formatter.FormatPorts(info.Labels),
			Status:    cStatus,
		}

		if tmpl != nil {
			var b bytes.Buffer
			if err := tmpl.Execute(&b, p); err != nil {
				return err
			}
			if _, err = fmt.Fprintf(w, b.String()+"\n"); err != nil {
				return err
			}
		} else if quiet {
			if _, err := fmt.Fprintf(w, "%s\n", id); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				p.ID,
				p.Image,
				p.Command,
				formatter.TimeSinceInHuman(info.CreatedAt),
				p.Status,
				p.Ports,
				p.Names,
			); err != nil {
				return err
			}
		}
	}
	if f, ok := w.(Flusher); ok {
		return f.Flush()
	}
	return nil
}
