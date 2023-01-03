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
	"text/tabwriter"

	"github.com/containerd/containerd"
	gocni "github.com/containerd/go-cni"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/containerutil"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/portutil"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

func newComposePsCommand() *cobra.Command {
	var composePsCommand = &cobra.Command{
		Use:           "ps [flags] [SERVICE...]",
		Short:         "List containers of services",
		RunE:          composePsAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	composePsCommand.Flags().String("format", "", "Format the output. Supported values: [json]")
	return composePsCommand
}

type composeContainerPrintable struct {
	ID       string
	Name     string
	Command  string
	Project  string
	Service  string
	State    string
	Health   string // placeholder, lack containerd support.
	ExitCode uint32
	// `Publishers` stores docker-compatible ports and used for json output.
	// `Ports` stores formatted ports and only used for console output.
	Publishers []PortPublisher
	Ports      string `json:"-"`
}

func composePsAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	if format != "json" && format != "" {
		return fmt.Errorf("unsupported format %s, supported formats are: [json]", format)
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	c, err := getComposer(cmd, client, globalOptions)
	if err != nil {
		return err
	}
	serviceNames, err := c.ServiceNames(args...)
	if err != nil {
		return err
	}
	containers, err := c.Containers(ctx, serviceNames...)
	if err != nil {
		return err
	}

	containersPrintable := make([]composeContainerPrintable, len(containers))
	eg, ctx := errgroup.WithContext(ctx)
	for i, container := range containers {
		i, container := i, container
		eg.Go(func() error {
			var p composeContainerPrintable
			var err error
			if format == "json" {
				p, err = composeContainerPrintableJSON(ctx, container)
			} else {
				p, err = composeContainerPrintableTab(ctx, container)
			}
			if err != nil {
				return err
			}
			containersPrintable[i] = p
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return err
	}

	if format == "json" {
		outJSON, err := formatter.ToJSON(containersPrintable, "", "")
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(cmd.OutOrStdout(), outJSON)
		return err
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 4, 8, 4, ' ', 0)
	fmt.Fprintln(w, "NAME\tCOMMAND\tSERVICE\tSTATUS\tPORTS")
	for _, p := range containersPrintable {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			p.Name,
			p.Command,
			p.Service,
			p.State,
			p.Ports,
		); err != nil {
			return err
		}
	}

	return w.Flush()
}

// composeContainerPrintableTab constructs composeContainerPrintable with fields
// only for console output.
func composeContainerPrintableTab(ctx context.Context, container containerd.Container) (composeContainerPrintable, error) {
	info, err := container.Info(ctx, containerd.WithoutRefreshedMetadata)
	if err != nil {
		return composeContainerPrintable{}, err
	}
	spec, err := container.Spec(ctx)
	if err != nil {
		return composeContainerPrintable{}, err
	}
	status := formatter.ContainerStatus(ctx, container)
	if status == "Up" {
		status = "running" // corresponds to Docker Compose v2.0.1
	}

	return composeContainerPrintable{
		Name:    info.Labels[labels.Name],
		Command: formatter.InspectContainerCommandTrunc(spec),
		Service: info.Labels[labels.ComposeService],
		State:   status,
		Ports:   formatter.FormatPorts(info.Labels),
	}, nil
}

// composeContainerPrintableTab constructs composeContainerPrintable with fields
// only for json output and compatible docker output.
func composeContainerPrintableJSON(ctx context.Context, container containerd.Container) (composeContainerPrintable, error) {
	info, err := container.Info(ctx, containerd.WithoutRefreshedMetadata)
	if err != nil {
		return composeContainerPrintable{}, err
	}
	spec, err := container.Spec(ctx)
	if err != nil {
		return composeContainerPrintable{}, err
	}

	var (
		state    string
		exitCode uint32
	)
	status, err := containerutil.ContainerStatus(ctx, container)
	if err == nil {
		// show exitCode only when container is exited/stopped
		if status.Status == containerd.Stopped {
			exitCode = status.ExitStatus
		}
		state = string(status.Status)
	} else {
		state = string(containerd.Unknown)
	}

	return composeContainerPrintable{
		ID:         container.ID(),
		Name:       info.Labels[labels.Name],
		Command:    formatter.InspectContainerCommand(spec, false, false),
		Project:    info.Labels[labels.ComposeProject],
		Service:    info.Labels[labels.ComposeService],
		State:      state,
		Health:     "",
		ExitCode:   exitCode,
		Publishers: formatPublishers(info.Labels),
	}, nil
}

// PortPublisher hold status about published port
// Use this to match the json output with docker compose
// FYI: https://github.com/docker/compose/blob/v2.13.0/pkg/api/api.go#L305C27-L311
type PortPublisher struct {
	URL           string
	TargetPort    int
	PublishedPort int
	Protocol      string
}

// formatPublishers parses and returns docker-compatible []PortPublisher from
// label map. If an error happens, an empty slice is returned.
func formatPublishers(labelMap map[string]string) []PortPublisher {
	mapper := func(pm gocni.PortMapping) PortPublisher {
		return PortPublisher{
			URL:           pm.HostIP,
			TargetPort:    int(pm.ContainerPort),
			PublishedPort: int(pm.HostPort),
			Protocol:      pm.Protocol,
		}
	}

	var dockerPorts []PortPublisher
	if portMappings, err := portutil.ParsePortsLabel(labelMap); err == nil {
		for _, p := range portMappings {
			dockerPorts = append(dockerPorts, mapper(p))
		}
	} else {
		logrus.Error(err.Error())
	}
	return dockerPorts
}
