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
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/runtime/restart"
	"github.com/containerd/errdefs"
	gocni "github.com/containerd/go-cni"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/compose"
	"github.com/containerd/nerdctl/v2/pkg/containerutil"
	"github.com/containerd/nerdctl/v2/pkg/formatter"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/portutil"
)

func newComposePsCommand() *cobra.Command {
	var composePsCommand = &cobra.Command{
		Use:           "ps [flags] [SERVICE...]",
		Short:         "List containers of services",
		RunE:          composePsAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	composePsCommand.Flags().String("format", "table", "Format the output. Supported values: [table|json]")
	composePsCommand.Flags().String("filter", "", "Filter matches containers based on given conditions")
	composePsCommand.Flags().StringArray("status", []string{}, "Filter services by status. Values: [paused | restarting | removing | running | dead | created | exited]")
	composePsCommand.Flags().BoolP("quiet", "q", false, "Only display container IDs")
	composePsCommand.Flags().Bool("services", false, "Display services")
	composePsCommand.Flags().BoolP("all", "a", false, "Show all containers (default shows just running)")
	return composePsCommand
}

type composeContainerPrintable struct {
	ID       string
	Name     string
	Image    string
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
	if format != "json" && format != "table" {
		return fmt.Errorf("unsupported format %s, supported formats are: [table|json]", format)
	}
	status, err := cmd.Flags().GetStringArray("status")
	if err != nil {
		return err
	}
	quiet, err := cmd.Flags().GetBool("quiet")
	if err != nil {
		return err
	}
	displayServices, err := cmd.Flags().GetBool("services")
	if err != nil {
		return err
	}
	filter, err := cmd.Flags().GetString("filter")
	if err != nil {
		return err
	}
	if filter != "" {
		splited := strings.SplitN(filter, "=", 2)
		if len(splited) != 2 {
			return fmt.Errorf("invalid argument \"%s\" for \"-f, --filter\": bad format of filter (expected name=value)", filter)
		}
		// currently only the 'status' filter is supported
		if splited[0] != "status" {
			return fmt.Errorf("invalid filter '%s'", splited[0])
		}
		status = append(status, splited[1])
	}

	all, err := cmd.Flags().GetBool("all")
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()
	options, err := getComposeOptions(cmd, globalOptions.DebugFull, globalOptions.Experimental)
	if err != nil {
		return err
	}
	c, err := compose.New(client, globalOptions, options, cmd.OutOrStdout(), cmd.ErrOrStderr())
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

	if !all {
		var upContainers []containerd.Container
		for _, container := range containers {
			// cStatus := formatter.ContainerStatus(ctx, c)
			cStatus, err := containerutil.ContainerStatus(ctx, container)
			if err != nil {
				continue
			}
			if cStatus.Status == containerd.Running {
				upContainers = append(upContainers, container)
			}
		}
		containers = upContainers
	}

	if len(status) != 0 {
		var filterdContainers []containerd.Container
		for _, container := range containers {
			cStatus := statusForFilter(ctx, container)
			for _, s := range status {
				if cStatus == s {
					filterdContainers = append(filterdContainers, container)
				}
			}
		}
		containers = filterdContainers
	}

	if quiet {
		for _, c := range containers {
			fmt.Fprintln(cmd.OutOrStdout(), c.ID())
		}
		return nil
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

	if displayServices {
		for _, p := range containersPrintable {
			fmt.Fprintln(cmd.OutOrStdout(), p.Service)
		}
		return nil
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
	fmt.Fprintln(w, "NAME\tIMAGE\tCOMMAND\tSERVICE\tSTATUS\tPORTS")
	for _, p := range containersPrintable {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			p.Name,
			p.Image,
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
	image, err := container.Image(ctx)
	if err != nil {
		return composeContainerPrintable{}, err
	}

	return composeContainerPrintable{
		Name:    info.Labels[labels.Name],
		Image:   image.Metadata().Name,
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
			state = "exited"
			exitCode = status.ExitStatus
		} else {
			state = string(status.Status)
		}
	} else {
		state = string(containerd.Unknown)
	}
	image, err := container.Image(ctx)
	if err != nil {
		return composeContainerPrintable{}, err
	}

	return composeContainerPrintable{
		ID:         container.ID(),
		Name:       info.Labels[labels.Name],
		Image:      image.Metadata().Name,
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
		log.L.Error(err.Error())
	}
	return dockerPorts
}

// statusForFilter returns the status value to be matched with the 'status' filter
func statusForFilter(ctx context.Context, c containerd.Container) string {
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
			return string(containerd.Created)
		}
		return string(containerd.Unknown)
	}

	status, err := task.Status(ctx)
	if err != nil {
		return string(containerd.Unknown)
	}
	labels, err := c.Labels(ctx)
	if err != nil {
		return string(containerd.Unknown)
	}

	switch s := status.Status; s {
	case containerd.Stopped:
		if labels[restart.StatusLabel] == string(containerd.Running) && restart.Reconcile(status, labels) {
			return "restarting"
		}
		return "exited"
	default:
		return string(s)
	}
}
