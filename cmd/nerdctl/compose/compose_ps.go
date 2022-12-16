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

package compose

import (
	"context"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/containerd/containerd"
	gocni "github.com/containerd/go-cni"
	ncclient "github.com/containerd/nerdctl/cmd/nerdctl/client"
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

type ContainerPrintable struct {
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
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	if format != "json" && format != "" {
		return fmt.Errorf("unsupported format %s, supported formats are: [json]", format)
	}

	client, ctx, cancel, err := ncclient.New(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	c, err := getComposer(cmd, client)
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

	containersPrintable := make([]ContainerPrintable, len(containers))
	eg, ctx := errgroup.WithContext(ctx)
	for i, container := range containers {
		i, container := i, container
		eg.Go(func() error {
			var p ContainerPrintable
			var err error
			if format == "json" {
				p, err = ContainerPrintableJSON(ctx, container)
			} else {
				p, err = ContainerPrintableTab(ctx, container)
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

// ContainerPrintableTab constructs ContainerPrintable with fields
// only for console output.
func ContainerPrintableTab(ctx context.Context, container containerd.Container) (ContainerPrintable, error) {
	info, err := container.Info(ctx, containerd.WithoutRefreshedMetadata)
	if err != nil {
		return ContainerPrintable{}, err
	}
	spec, err := container.Spec(ctx)
	if err != nil {
		return ContainerPrintable{}, err
	}
	status := formatter.ContainerStatus(ctx, container)
	if status == "Up" {
		status = "running" // corresponds to Docker Compose v2.0.1
	}

	return ContainerPrintable{
		Name:    info.Labels[labels.Name],
		Command: formatter.InspectContainerCommandTrunc(spec),
		Service: info.Labels[labels.ComposeService],
		State:   status,
		Ports:   formatter.FormatPorts(info.Labels),
	}, nil
}

// ContainerPrintableTab constructs ContainerPrintable with fields
// only for json output and compatible docker output.
func ContainerPrintableJSON(ctx context.Context, container containerd.Container) (ContainerPrintable, error) {
	info, err := container.Info(ctx, containerd.WithoutRefreshedMetadata)
	if err != nil {
		return ContainerPrintable{}, err
	}
	spec, err := container.Spec(ctx)
	if err != nil {
		return ContainerPrintable{}, err
	}

	var (
		state    string
		exitCode uint32
	)
	status, err := containerStatus(ctx, container)
	if err == nil {
		// show exitCode only when container is exited/stopped
		if status.Status == containerd.Stopped {
			exitCode = status.ExitStatus
		}
		state = string(status.Status)
	} else {
		state = string(containerd.Unknown)
	}

	return ContainerPrintable{
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

func containerStatus(ctx context.Context, c containerd.Container) (containerd.Status, error) {
	// Just in case, there is something wrong in server.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	task, err := c.Task(ctx, nil)
	if err != nil {
		return containerd.Status{}, err
	}

	return task.Status(ctx)
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
