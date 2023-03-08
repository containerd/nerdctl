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

package container

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"
	"text/template"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/pkg/progress"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/labels/k8slabels"
	"github.com/sirupsen/logrus"
)

// List prints containers according to `options`.
func List(ctx context.Context, client *containerd.Client, options types.ContainerListOptions) error {
	containers, err := filterContainers(ctx, client, options.Filters, options.LastN, options.All)
	if err != nil {
		return err
	}
	return printContainers(ctx, client, containers, options)
}

// filterContainers returns containers matching the filters.
//
//   - Supported filters: https://github.com/containerd/nerdctl/blob/main/docs/command-reference.md#whale-blue_square-nerdctl-ps
//   - all means showing all containers (default shows just running).
//   - lastN means only showing n last created containers (includes all states). Non-positive values are ignored.
//     In other words, if lastN is positive, all will be set to true.
func filterContainers(ctx context.Context, client *containerd.Client, filters []string, lastN int, all bool) ([]containerd.Container, error) {
	containers, err := client.Containers(ctx)
	if err != nil {
		return nil, err
	}
	filterCtx, err := foldContainerFilters(ctx, containers, filters)
	if err != nil {
		return nil, err
	}
	containers = filterCtx.MatchesFilters(ctx)
	if lastN > 0 {
		all = true
		sort.Slice(containers, func(i, j int) bool {
			infoI, _ := containers[i].Info(ctx, containerd.WithoutRefreshedMetadata)
			infoJ, _ := containers[j].Info(ctx, containerd.WithoutRefreshedMetadata)
			return infoI.CreatedAt.After(infoJ.CreatedAt)
		})
		if lastN < len(containers) {
			containers = containers[:lastN]
		}
	}

	if all {
		return containers, nil
	}
	var upContainers []containerd.Container
	for _, c := range containers {
		cStatus := formatter.ContainerStatus(ctx, c)
		if strings.HasPrefix(cStatus, "Up") {
			upContainers = append(upContainers, c)
		}
	}
	return upContainers, nil
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
	Size      string
	Labels    string
	// TODO: "LocalVolumes", "Mounts", "Networks", "RunningFor", "State"
}

func printContainers(ctx context.Context, client *containerd.Client, containers []containerd.Container, options types.ContainerListOptions) error {
	w := options.Stdout
	var (
		wide bool
		tmpl *template.Template
	)
	switch options.Format {
	case "", "table":
		w = tabwriter.NewWriter(w, 4, 8, 4, ' ', 0)
		if !options.Quiet {
			printHeader := "CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES"
			if options.Size {
				printHeader += "\tSIZE"
			}
			fmt.Fprintln(w, printHeader)
		}
	case "raw":
		return errors.New("unsupported format: \"raw\"")
	case "wide":
		w = tabwriter.NewWriter(w, 4, 8, 4, ' ', 0)
		if !options.Quiet {
			fmt.Fprintln(w, "CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES\tRUNTIME\tPLATFORM\tSIZE")
			wide = true
		}
	default:
		if options.Quiet {
			return errors.New("format and quiet must not be specified together")
		}
		var err error
		tmpl, err = formatter.ParseTemplate(options.Format)
		if err != nil {
			return err
		}
	}

	for _, c := range containers {
		info, err := c.Info(ctx, containerd.WithoutRefreshedMetadata)
		if err != nil {
			if errdefs.IsNotFound(err) {
				logrus.Warn(err)
				continue
			}
			return err
		}

		spec, err := c.Spec(ctx)
		if err != nil {
			if errdefs.IsNotFound(err) {
				logrus.Warn(err)
				continue
			}
			return err
		}

		imageName := info.Image
		id := c.ID()
		if options.Truncate && len(id) > 12 {
			id = id[:12]
		}

		p := containerPrintable{
			Command:   formatter.InspectContainerCommand(spec, options.Truncate, true),
			CreatedAt: info.CreatedAt.Round(time.Second).Local().String(), // format like "2021-08-07 02:19:45 +0900 JST"
			ID:        id,
			Image:     imageName,
			Platform:  info.Labels[labels.Platform],
			Names:     getPrintableContainerName(info.Labels),
			Ports:     formatter.FormatPorts(info.Labels),
			Status:    formatter.ContainerStatus(ctx, c),
			Runtime:   info.Runtime.Name,
			Labels:    formatter.FormatLabels(info.Labels),
		}

		if options.Size || wide {
			containerSize, err := getContainerSize(ctx, client, c, info)
			if err != nil {
				return err
			}
			p.Size = containerSize
		}

		if tmpl != nil {
			var b bytes.Buffer
			if err := tmpl.Execute(&b, p); err != nil {
				return err
			}
			if _, err = fmt.Fprintf(w, b.String()+"\n"); err != nil {
				return err
			}
		} else if options.Quiet {
			if _, err := fmt.Fprintf(w, "%s\n", id); err != nil {
				return err
			}
		} else {
			format := "%s\t%s\t%s\t%s\t%s\t%s\t%s"
			args := []interface{}{
				p.ID,
				p.Image,
				p.Command,
				formatter.TimeSinceInHuman(info.CreatedAt),
				p.Status,
				p.Ports,
				p.Names,
			}
			if wide {
				format += "\t%s\t%s\t%s\n"
				args = append(args, p.Runtime, p.Platform, p.Size)
			} else if options.Size {
				format += "\t%s\n"
				args = append(args, p.Size)
			} else {
				format += "\n"
			}
			if _, err := fmt.Fprintf(w, format, args...); err != nil {
				return err
			}
		}

	}
	if f, ok := w.(formatter.Flusher); ok {
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
			}
			// Pod sandbox
			return fmt.Sprintf("k8s://%s/%s", ns, podName)
		}
	}
	return ""
}

type containerVolume struct {
	Type        string
	Name        string
	Source      string
	Destination string
	Mode        string
	RW          bool
	Propagation string
}

func getContainerVolumes(containerLabels map[string]string) []*containerVolume {
	var vols []*containerVolume
	volLabels := []string{labels.AnonymousVolumes, labels.Mounts}
	for _, volLabel := range volLabels {
		names, ok := containerLabels[volLabel]
		if !ok {
			continue
		}
		var (
			volumes []*containerVolume
			err     error
		)
		if volLabel == labels.Mounts {
			err = json.Unmarshal([]byte(names), &volumes)
		}
		if volLabel == labels.AnonymousVolumes {
			var anonymous []string
			err = json.Unmarshal([]byte(names), &anonymous)
			for _, anony := range anonymous {
				volumes = append(volumes, &containerVolume{Name: anony})
			}

		}
		if err != nil {
			logrus.Warn(err)
		}
		vols = append(vols, volumes...)
	}
	return vols
}

func getContainerNetworks(containerLables map[string]string) []string {
	var networks []string
	if names, ok := containerLables[labels.Networks]; ok {
		if err := json.Unmarshal([]byte(names), &networks); err != nil {
			logrus.Warn(err)
		}
	}
	return networks
}

func getContainerSize(ctx context.Context, client *containerd.Client, c containerd.Container, info containers.Container) (string, error) {
	// get container snapshot size
	snapshotKey := info.SnapshotKey
	var containerSize int64

	if snapshotKey != "" {
		usage, err := client.SnapshotService(info.Snapshotter).Usage(ctx, snapshotKey)
		if err != nil {
			return "", err
		}
		containerSize = usage.Size
	}

	// get the image interface
	image, err := c.Image(ctx)
	if err != nil {
		return "", err
	}

	imageSize, err := imgutil.UnpackedImageSize(ctx, image)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s (virtual %s)", progress.Bytes(containerSize).String(), progress.Bytes(imageSize).String()), nil
}
