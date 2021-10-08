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

package composer

import (
	"bytes"
	"context"
	"fmt"
	"text/tabwriter"
	"text/template"

	"github.com/compose-spec/compose-go/types"
	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/docker/cli/templates"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

type PsOptions struct{}

type containerPs struct {
	Name        string
	ServiceName string
	Container   containerd.Container
}

func (c *Composer) Ps(ctx context.Context, clicontext *cli.Context, po PsOptions, client *containerd.Client) error {
	containers := []containerPs{}

	if err := c.project.WithServices(nil, func(svc types.ServiceConfig) error {
		ps, err := serviceparser.Parse(c.project, svc)
		if err != nil {
			return err
		}
		for _, container := range ps.Containers {
			containersGot, _ := client.Containers(ctx, fmt.Sprintf("labels.%q==%s", labels.Name, container.Name))
			if len(containersGot) != 1 {
				return errors.New(fmt.Sprintf("Expected 1 container, got %d", len(containersGot)))
			}

			containers = append(containers, containerPs{
				ServiceName: ps.Unparsed.Name,
				Name:        container.Name,
				Container:   containersGot[0],
			})
		}
		return nil
	}); err != nil {
		return err
	}

	return c.ps(ctx, clicontext, containers)
}

type containerPrintable struct {
	Name    string
	Command string
	Service string
	Status  string
	Ports   string
}

type Flusher interface {
	Flush() error
}

func (c *Composer) ps(ctx context.Context, clicontext *cli.Context, containers []containerPs) error {
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

	for _, container := range containers {
		info, err := container.Container.Info(ctx, containerd.WithoutRefreshedMetadata)
		if err != nil {
			return err
		}

		spec, err := container.Container.Spec(ctx)
		if err != nil {
			return err
		}

		p := containerPrintable{
			Name:    container.Name,
			Command: formatter.InspectContainerCommandTrunc(spec),
			Service: container.ServiceName,
			Status:  "running",
			Ports:   formatter.FormatPorts(info.Labels),
		}

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
