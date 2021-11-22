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
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"text/template"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/labels/k8slabels"

	"github.com/spf13/cobra"
)

func newPsCommand() *cobra.Command {
	var psCommand = &cobra.Command{
		Use:           "ps",
		Args:          cobra.NoArgs,
		Short:         "List containers",
		RunE:          psAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	psCommand.Flags().BoolP("all", "a", false, "Show all containers (default shows just running)")
	psCommand.Flags().Bool("no-trunc", false, "Don't truncate output")
	psCommand.Flags().BoolP("quiet", "q", false, "Only display container IDs")
	// Alias "-f" is reserved for "--filter"
	psCommand.Flags().String("format", "", "Format the output using the given Go template, e.g, '{{json .}}', 'wide'")
	psCommand.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json", "table", "wide"}, cobra.ShellCompDirectiveNoFileComp
	})
	return psCommand
}

func psAction(cmd *cobra.Command, args []string) error {
	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()
	containers, err := client.Containers(ctx)
	if err != nil {
		return err
	}
	return printContainers(ctx, cmd, containers)
}

type containerPrintable struct {
	Command   string
	CreatedAt string
	ID        string
	Image     string
	Platform  string // nerdctl extension
	Names     string
	Ports     string
	Status    string
	Runtime   string // nerdctl extension
	// TODO: "Labels", "LocalVolumes", "Mounts", "Networks", "RunningFor", "Size", "State"
}

func printContainers(ctx context.Context, cmd *cobra.Command, containers []containerd.Container) error {
	noTrunc, err := cmd.Flags().GetBool("no-trunc")
	if err != nil {
		return err
	}
	var wide bool
	trunc := !noTrunc
	all, err := cmd.Flags().GetBool("all")
	if err != nil {
		return err
	}
	quiet, err := cmd.Flags().GetBool("quiet")
	if err != nil {
		return err
	}
	var w io.Writer
	w = os.Stdout
	var tmpl *template.Template
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	switch format {
	case "", "table":
		w = tabwriter.NewWriter(os.Stdout, 4, 8, 4, ' ', 0)
		if !quiet {
			fmt.Fprintln(w, "CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
		}
	case "raw":
		return errors.New("unsupported format: \"raw\"")
	case "wide":
		w = tabwriter.NewWriter(os.Stdout, 4, 8, 4, ' ', 0)
		if !quiet {
			fmt.Fprintln(w, "CONTAINER ID\tIMAGE\tPLATFORM\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES\tRUNTIME")
			wide = true
		}
	default:
		if quiet {
			return errors.New("format and quiet must not be specified together")
		}
		var err error
		tmpl, err = parseTemplate(format)
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
			Platform:  info.Labels[labels.Platform],
			Names:     getPrintableContainerName(info.Labels),
			Ports:     formatter.FormatPorts(info.Labels),
			Status:    cStatus,
			Runtime:   info.Runtime.Name,
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
			if wide {
				if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					p.ID,
					p.Image,
					p.Platform,
					p.Command,
					formatter.TimeSinceInHuman(info.CreatedAt),
					p.Status,
					p.Ports,
					p.Names,
					p.Runtime,
				); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t\n",
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
	}
	if f, ok := w.(Flusher); ok {
		return f.Flush()
	}
	return nil
}

func getPrintableContainerName(containerLabels map[string]string) string {
	if name, ok := containerLabels[labels.Name]; ok {
		return name
	}

	if ns, ok := containerLabels[k8slabels.PodNamespace]; ok {
		if podName, ok := containerLabels[k8slabels.PodName]; ok {
			if containerName, ok := containerLabels[k8slabels.ContainerName]; ok {
				// Container
				return fmt.Sprintf("k8s://%s/%s/%s", ns, podName, containerName)
			} else {
				// Pod sandbox
				return fmt.Sprintf("k8s://%s/%s", ns, podName)
			}
		}
	}
	return ""
}
