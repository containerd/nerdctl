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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/oci"
	gocni "github.com/containerd/go-cni"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/docker/go-units"
	"github.com/sirupsen/logrus"
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
	all := clicontext.Bool("all")
	quiet := clicontext.Bool("quiet")

	w := tabwriter.NewWriter(clicontext.App.Writer, 4, 8, 4, ' ', 0)
	if !quiet {
		fmt.Fprintln(w, "CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
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

		cStatus := containerStatus(ctx, c)
		if !strings.HasPrefix(cStatus, "Up") && !all {
			continue
		}

		if quiet {
			if _, err := fmt.Fprintf(w, "%s\n", id); err != nil {
				return err
			}
			continue
		}

		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			id,
			imageName,
			inspectContainerCommand(spec, trunc),
			timeSinceInHuman(info.CreatedAt),
			cStatus,
			formatPorts(info.Labels),
			info.Labels[labels.Name],
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
		return fmt.Sprintf("Exited (%v) %s", status.ExitStatus, timeSinceInHuman(status.ExitTime))
	case containerd.Running:
		return "Up" // TODO: print "status.UpTime" (inexistent yet)
	default:
		return strings.Title(string(s))
	}
}

func inspectContainerCommand(spec *oci.Spec, trunc bool) string {
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

func formatPorts(labelMap map[string]string) string {
	portsJSON := labelMap[labels.Ports]
	if portsJSON == "" {
		return ""
	}
	var ports []gocni.PortMapping
	if err := json.Unmarshal([]byte(portsJSON), &ports); err != nil {
		logrus.WithError(err).Errorf("failed to parse label %q=%q", labels.Ports, portsJSON)
		return ""
	}
	if len(ports) == 0 {
		return ""
	}
	strs := make([]string, len(ports))
	for i, p := range ports {
		strs[i] = fmt.Sprintf("%s:%d->%d/%s", p.HostIP, p.HostPort, p.ContainerPort, p.Protocol)
	}
	return strings.Join(strs, ", ")
}
