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
	"fmt"
	"text/tabwriter"
	"text/template"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/docker/cli/templates"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

var composePsCommand = &cli.Command{
	Name:   "ps",
	Usage:  "List containers of services",
	Action: composePsAction,
	Flags:  []cli.Flag{},
}

func composePsAction(clicontext *cli.Context) error {
	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	c, err := getComposer(clicontext, client)
	if err != nil {
		return err
	}
	services, err := c.Services(ctx)
	if err != nil {
		return err
	}

	type containerPrintable struct {
		Name    string
		Command string
		Service string
		Status  string
		Ports   string
	}

	var containersPrintable []containerPrintable

	for _, svc := range services {
		for _, container := range svc.Containers {
			containersGot, err := client.Containers(ctx, fmt.Sprintf("labels.%q==%s", labels.Name, container.Name))
			if err != nil {
				return err
			}
			if len(containersGot) != 1 {
				return fmt.Errorf("expected 1 container, got %d", len(containersGot))
			}
			info, err := containersGot[0].Info(ctx, containerd.WithoutRefreshedMetadata)
			if err != nil {
				return err
			}

			spec, err := containersGot[0].Spec(ctx)
			if err != nil {
				return err
			}
			p := containerPrintable{
				Name:    container.Name,
				Command: formatter.InspectContainerCommandTrunc(spec),
				Service: svc.Unparsed.Name,
				Status:  "running", // FIXME
				Ports:   formatter.FormatPorts(info.Labels),
			}
			containersPrintable = append(containersPrintable, p)
		}
	}

	w := clicontext.App.Writer
	var tmpl *template.Template
	switch format := clicontext.String("format"); format {
	case "", "table":
		w = tabwriter.NewWriter(clicontext.App.Writer, 4, 8, 4, ' ', 0)
		fmt.Fprintln(w, "NAME\tCOMMAND\tSERVICE\tSTATUS\tPORTS")
	case "raw":
		return errors.New("unsupported format: \"raw\"")
	default:
		var err error
		tmpl, err = templates.Parse(format)
		if err != nil {
			return err
		}
	}

	for _, p := range containersPrintable {
		if tmpl != nil {
			var b bytes.Buffer
			if err := tmpl.Execute(&b, p); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, b.String()+"\n"); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				p.Name,
				p.Command,
				p.Service,
				p.Status,
				p.Ports,
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
