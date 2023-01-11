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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"text/template"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/pkg/progress"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/labels/k8slabels"
	"github.com/opencontainers/image-spec/identity"
	"github.com/sirupsen/logrus"

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
	psCommand.Flags().IntP("last", "n", -1, "Show n last created containers (includes all states)")
	psCommand.Flags().BoolP("latest", "l", false, "Show the latest created container (includes all states)")
	psCommand.Flags().Bool("no-trunc", false, "Don't truncate output")
	psCommand.Flags().BoolP("quiet", "q", false, "Only display container IDs")
	psCommand.Flags().BoolP("size", "s", false, "Display total file sizes")

	// Alias "-f" is reserved for "--filter"
	psCommand.Flags().String("format", "", "Format the output using the given Go template, e.g, '{{json .}}', 'wide'")
	psCommand.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json", "table", "wide"}, cobra.ShellCompDirectiveNoFileComp
	})
	psCommand.Flags().StringSliceP("filter", "f", nil, "Filter matches containers based on given conditions")
	return psCommand
}

func psAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()
	all, err := cmd.Flags().GetBool("all")
	if err != nil {
		return err
	}
	latest, err := cmd.Flags().GetBool("latest")
	if err != nil {
		return err
	}
	lastN, err := cmd.Flags().GetInt("last")
	if err != nil {
		return err
	}
	if lastN == -1 && latest {
		lastN = 1
	}
	containers, err := client.Containers(ctx)
	if err != nil {
		return err
	}
	filters, err := cmd.Flags().GetStringSlice("filter")
	if err != nil {
		return err
	}
	filterCtx, err := foldContainerFilters(ctx, containers, filters)
	if err != nil {
		return err
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
	return printContainers(ctx, client, cmd, containers, all)
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

func printContainers(ctx context.Context, client *containerd.Client, cmd *cobra.Command, containers []containerd.Container, all bool) error {
	noTrunc, err := cmd.Flags().GetBool("no-trunc")
	if err != nil {
		return err
	}
	var wide bool
	trunc := !noTrunc

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

	size := false
	if !quiet {
		size, err = cmd.Flags().GetBool("size")
		if err != nil {
			return err
		}
	}

	switch format {
	case "", "table":
		w = tabwriter.NewWriter(os.Stdout, 4, 8, 4, ' ', 0)
		if !quiet {
			printHeader := "CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES"
			if size {
				printHeader += "\tSIZE"
			}
			fmt.Fprintln(w, printHeader)
		}
	case "raw":
		return errors.New("unsupported format: \"raw\"")
	case "wide":
		w = tabwriter.NewWriter(os.Stdout, 4, 8, 4, ' ', 0)
		if !quiet {
			fmt.Fprintln(w, "CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES\tRUNTIME\tPLATFORM\tSIZE")
			wide = true
		}
	default:
		if quiet {
			return errors.New("format and quiet must not be specified together")
		}
		var err error
		tmpl, err = formatter.ParseTemplate(format)
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
		if trunc && len(id) > 12 {
			id = id[:12]
		}

		cStatus := formatter.ContainerStatus(ctx, c)
		if !strings.HasPrefix(cStatus, "Up") && !all {
			continue
		}

		p := containerPrintable{
			Command:   formatter.InspectContainerCommand(spec, trunc, true),
			CreatedAt: info.CreatedAt.Round(time.Second).Local().String(), // format like "2021-08-07 02:19:45 +0900 JST"
			ID:        id,
			Image:     imageName,
			Platform:  info.Labels[labels.Platform],
			Names:     getPrintableContainerName(info.Labels),
			Ports:     formatter.FormatPorts(info.Labels),
			Status:    cStatus,
			Runtime:   info.Runtime.Name,
			Labels:    formatter.FormatLabels(info.Labels),
		}

		if size || wide {
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
		} else if quiet {
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
			} else if size {
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

	sn := client.SnapshotService(info.Snapshotter)

	imageSize, err := unpackedImageSize(ctx, sn, image)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s (virtual %s)", progress.Bytes(containerSize).String(), progress.Bytes(imageSize).String()), nil
}

// TODO: this code should be remove to a common package after the refactor completed
type snapshotKey string

// recursive function to calculate total usage of key's parent
func (key snapshotKey) add(ctx context.Context, s snapshots.Snapshotter, usage *snapshots.Usage) error {
	if key == "" {
		return nil
	}
	u, err := s.Usage(ctx, string(key))
	if err != nil {
		return err
	}

	usage.Add(u)

	info, err := s.Stat(ctx, string(key))
	if err != nil {
		return err
	}

	key = snapshotKey(info.Parent)
	return key.add(ctx, s, usage)
}

// unpackedImageSize is the size of the unpacked snapshots.
// Does not contain the size of the blobs in the content store. (Corresponds to Docker).
func unpackedImageSize(ctx context.Context, s snapshots.Snapshotter, img containerd.Image) (int64, error) {
	diffIDs, err := img.RootFS(ctx)
	if err != nil {
		return 0, err
	}

	chainID := identity.ChainID(diffIDs).String()
	usage, err := s.Usage(ctx, chainID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			logrus.WithError(err).Debugf("image %q seems not unpacked", img.Name())
			return 0, nil
		}
		return 0, err
	}

	info, err := s.Stat(ctx, chainID)
	if err != nil {
		return 0, err
	}

	//add ChainID's parent usage to the total usage
	if err := snapshotKey(info.Parent).add(ctx, s, &usage); err != nil {
		return 0, err
	}
	return usage.Size, nil
}
