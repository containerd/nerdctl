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
	"fmt"
	"text/tabwriter"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/spf13/cobra"
)

func newComposePsCommand() *cobra.Command {
	var composePsCommand = &cobra.Command{
		Use:           "ps",
		Short:         "List containers of services",
		RunE:          composePsAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	return composePsCommand
}

func composePsAction(cmd *cobra.Command, args []string) error {
	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	c, err := getComposer(cmd, client)
	if err != nil {
		return err
	}

	// TODO: make JSON-printable.
	// The JSON format must correspond to `docker compose ps --json` (Docker Compose v2)
	type containerPrintable struct {
		Name    string
		Command string
		Service string
		Status  string
		Ports   string
	}

	containersPrintable := []containerPrintable{}

	serviceNames, err := c.ServiceNames(args...)
	if err != nil {
		return err
	}

	containers, err := c.Containers(ctx, serviceNames...)
	if err != nil {
		return err
	}
	for _, container := range containers {
		info, err := container.Info(ctx, containerd.WithoutRefreshedMetadata)
		if err != nil {
			return err
		}

		spec, err := container.Spec(ctx)
		if err != nil {
			return err
		}
		status := formatter.ContainerStatus(ctx, container)
		if status == "Up" {
			status = "running" // corresponds to Docker Compose v2.0.1
		}
		p := containerPrintable{
			Name:    info.Labels[labels.Name],
			Command: formatter.InspectContainerCommandTrunc(spec),
			Service: info.Labels[labels.ComposeService],
			Status:  status,
			Ports:   formatter.FormatPorts(info.Labels),
		}
		containersPrintable = append(containersPrintable, p)
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 4, 8, 4, ' ', 0)
	fmt.Fprintln(w, "NAME\tCOMMAND\tSERVICE\tSTATUS\tPORTS")

	for _, p := range containersPrintable {
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

	return w.Flush()
}
