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

package system

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"text/template"

	"github.com/docker/go-units"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/cmd/builder"
	"github.com/containerd/nerdctl/v2/pkg/cmd/container"
	"github.com/containerd/nerdctl/v2/pkg/cmd/image"
	"github.com/containerd/nerdctl/v2/pkg/cmd/volume"
	"github.com/containerd/nerdctl/v2/pkg/formatter"
	"github.com/containerd/nerdctl/v2/pkg/idgen"
)

// Df shows how much disk space containerd uses for the images, containers and volumes of the current
// namespace, plus the BuildKit build cache.
func Df(ctx context.Context, client *containerd.Client, options types.SystemDfOptions) error {
	du, err := DiskUsage(ctx, client, options)
	if err != nil {
		return err
	}
	return printDiskUsage(du, options)
}

// DiskUsage collects the disk usage of every kind of resource nerdctl manages.
func DiskUsage(ctx context.Context, client *containerd.Client, options types.SystemDfOptions) (types.DiskUsage, error) {
	du := types.DiskUsage{}

	var err error
	if du.Images, err = image.DiskUsage(ctx, client, options.GOptions, options.Verbose); err != nil {
		return du, err
	}
	if du.Containers, err = container.DiskUsage(ctx, client, options.GOptions, options.Verbose); err != nil {
		return du, err
	}
	if du.Volumes, err = volume.DiskUsage(ctx, client, options.GOptions, options.Verbose); err != nil {
		return du, err
	}

	// BuildKit is optional. When it is not reachable, the build cache is reported as empty rather
	// than omitted, so that the shape of the output does not depend on the daemons that happen to
	// be running.
	if options.BuildKitHost != "" {
		du.BuildCache, err = builder.DiskUsage(ctx, types.BuilderDiskUsageOptions{
			Stderr:       options.Stderr,
			GOptions:     options.GOptions,
			BuildKitHost: options.BuildKitHost,
			Verbose:      options.Verbose,
		})
		if err != nil {
			log.G(ctx).WithError(err).Warn("failed to get the build cache disk usage")
			du.BuildCache = types.BuildCacheDiskUsage{}
		}
	}

	return du, nil
}

// dfPrintable is a row of the summary table.
type dfPrintable struct {
	Type        string
	TotalCount  string
	Active      string
	Size        string
	Reclaimable string
}

// dfVerbosePrintable is what a `--format` template gets in verbose mode, mirroring Docker.
type dfVerbosePrintable struct {
	Images     []dfImagePrintable
	Containers []dfContainerPrintable
	Volumes    []dfVolumePrintable
	BuildCache []dfBuildCachePrintable
}

type dfImagePrintable struct {
	Repository   string
	Tag          string
	ID           string
	CreatedSince string
	Size         string
	SharedSize   string
	UniqueSize   string
	Containers   string
}

type dfContainerPrintable struct {
	ID           string
	Image        string
	Command      string
	LocalVolumes string
	Size         string
	RunningFor   string
	Status       string
	Names        string
}

type dfVolumePrintable struct {
	Name  string
	Links string
	Size  string
}

type dfBuildCachePrintable struct {
	ID            string
	CacheType     string
	Size          string
	CreatedSince  string
	LastUsedSince string
	UsageCount    string
	InUse         string
	Shared        string
}

// dfFormat is how the output was asked to be rendered.
type dfFormat struct {
	// tmpl is the template of a `--format`, or nil for the default columns.
	tmpl *template.Template
	// header renders the column labels of tmpl, leaving them intact.
	header *template.Template
	// table tells whether the output is a table: its columns are aligned under a header, and its
	// identifiers are shortened because it is meant to be read rather than parsed.
	table bool
}

func printDiskUsage(du types.DiskUsage, options types.SystemDfOptions) error {
	var (
		format dfFormat
		err    error
	)
	switch {
	case options.Format == "", options.Format == "table":
		// The default columns, rendered below.
		format.table = true
	case options.Format == "raw":
		return errors.New("unsupported format: \"raw\"")
	case formatter.IsTableFormat(options.Format):
		// `table {{.Type}}\t{{.Size}}` picks the columns but keeps the header and the alignment.
		format.table = true
		format.tmpl, format.header, err = formatter.ParseTableTemplate(options.Format)
	default:
		format.tmpl, err = formatter.ParseTemplate(options.Format)
	}
	if err != nil {
		return err
	}

	if options.Verbose {
		return printVerbose(du, options.Stdout, format)
	}
	return printSummary(du, options.Stdout, format)
}

// dfHeader labels the columns of the summary. A table format renders its header by running the very
// same template over it, so that the header always describes the columns that were asked for.
var dfHeader = dfPrintable{
	Type:        "TYPE",
	TotalCount:  "TOTAL",
	Active:      "ACTIVE",
	Size:        "SIZE",
	Reclaimable: "RECLAIMABLE",
}

func printSummary(du types.DiskUsage, stdout io.Writer, format dfFormat) error {
	rows := []dfPrintable{
		{
			Type:        "Images",
			TotalCount:  strconv.FormatInt(du.Images.TotalCount, 10),
			Active:      strconv.FormatInt(du.Images.ActiveCount, 10),
			Size:        humanSize(du.Images.TotalSize),
			Reclaimable: humanReclaimable(du.Images.Reclaimable, du.Images.TotalSize),
		},
		{
			Type:        "Containers",
			TotalCount:  strconv.FormatInt(du.Containers.TotalCount, 10),
			Active:      strconv.FormatInt(du.Containers.ActiveCount, 10),
			Size:        humanSize(du.Containers.TotalSize),
			Reclaimable: humanReclaimable(du.Containers.Reclaimable, du.Containers.TotalSize),
		},
		{
			Type:        "Local Volumes",
			TotalCount:  strconv.FormatInt(du.Volumes.TotalCount, 10),
			Active:      strconv.FormatInt(du.Volumes.ActiveCount, 10),
			Size:        humanSize(du.Volumes.TotalSize),
			Reclaimable: humanReclaimable(du.Volumes.Reclaimable, du.Volumes.TotalSize),
		},
		{
			Type:       "Build Cache",
			TotalCount: strconv.FormatInt(du.BuildCache.TotalCount, 10),
			Active:     strconv.FormatInt(du.BuildCache.ActiveCount, 10),
			Size:       humanSize(du.BuildCache.TotalSize),
			// Unlike the other kinds, Docker never shows a percentage for the build cache.
			Reclaimable: humanSize(du.BuildCache.Reclaimable),
		},
	}

	if format.tmpl != nil && !format.table {
		for _, row := range rows {
			if err := executeTemplate(stdout, format.tmpl, row); err != nil {
				return err
			}
		}
		return nil
	}

	w := newTabWriter(stdout)
	if format.tmpl != nil {
		if err := executeTemplate(w, format.header, dfHeader); err != nil {
			return err
		}
		for _, row := range rows {
			if err := executeTemplate(w, format.tmpl, row); err != nil {
				return err
			}
		}
		return w.Flush()
	}

	fmt.Fprintln(w, "TYPE\tTOTAL\tACTIVE\tSIZE\tRECLAIMABLE")
	for _, row := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", row.Type, row.TotalCount, row.Active, row.Size, row.Reclaimable)
	}
	return w.Flush()
}

func printVerbose(du types.DiskUsage, stdout io.Writer, format dfFormat) error {
	verbose := dfVerbosePrintable{}

	// Docker only shortens the identifiers for its table output (see Format.IsTable in
	// docker/cli), so that a custom format stays usable to look a resource up. A table format is
	// still a table, whatever columns it asks for.
	trunc := format.table

	for _, item := range du.Images.Items {
		repository, tag := item.Repository, item.Tag
		if repository == "" {
			repository = "<none>"
		}
		if tag == "" {
			tag = "<none>"
		}
		verbose.Images = append(verbose.Images, dfImagePrintable{
			Repository:   repository,
			Tag:          tag,
			ID:           displayID(item.ID, trunc),
			CreatedSince: formatter.TimeSinceInHuman(item.CreatedAt),
			Size:         humanSize(item.Size),
			SharedSize:   humanSize(item.SharedSize),
			UniqueSize:   humanSize(item.Size - item.SharedSize),
			Containers:   strconv.FormatInt(item.Containers, 10),
		})
	}

	for _, item := range du.Containers.Items {
		verbose.Containers = append(verbose.Containers, dfContainerPrintable{
			ID:           displayID(item.ID, trunc),
			Image:        item.Image,
			Command:      item.Command,
			LocalVolumes: strconv.FormatInt(item.LocalVolumes, 10),
			Size:         humanSize(item.SizeRw),
			RunningFor:   formatter.TimeSinceInHuman(item.CreatedAt),
			Status:       item.Status,
			Names:        item.Names,
		})
	}

	for _, item := range du.Volumes.Items {
		verbose.Volumes = append(verbose.Volumes, dfVolumePrintable{
			Name:  item.Name,
			Links: strconv.FormatInt(item.Links, 10),
			Size:  humanSize(item.Size),
		})
	}

	for _, item := range du.BuildCache.Items {
		lastUsedSince := ""
		if item.LastUsedAt != nil {
			lastUsedSince = formatter.TimeSinceInHuman(*item.LastUsedAt)
		}
		// Docker has no column for it, it marks the ID of a record in use with a star instead.
		id := displayID(item.ID, trunc)
		if item.InUse {
			id += "*"
		}
		verbose.BuildCache = append(verbose.BuildCache, dfBuildCachePrintable{
			ID:            id,
			CacheType:     item.CacheType,
			Size:          humanSize(item.Size),
			CreatedSince:  formatter.TimeSinceInHuman(item.CreatedAt),
			LastUsedSince: lastUsedSince,
			UsageCount:    strconv.Itoa(item.UsageCount),
			InUse:         strconv.FormatBool(item.InUse),
			Shared:        strconv.FormatBool(item.Shared),
		})
	}

	if format.tmpl != nil {
		return executeTemplate(stdout, format.tmpl, verbose)
	}

	fmt.Fprint(stdout, "Images space usage:\n\n")
	w := newTabWriter(stdout)
	fmt.Fprintln(w, "REPOSITORY\tTAG\tIMAGE ID\tCREATED\tSIZE\tSHARED SIZE\tUNIQUE SIZE\tCONTAINERS")
	for _, p := range verbose.Images {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			p.Repository, p.Tag, p.ID, p.CreatedSince, p.Size, p.SharedSize, p.UniqueSize, p.Containers)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	fmt.Fprint(stdout, "\nContainers space usage:\n\n")
	w = newTabWriter(stdout)
	fmt.Fprintln(w, "CONTAINER ID\tIMAGE\tCOMMAND\tLOCAL VOLUMES\tSIZE\tCREATED\tSTATUS\tNAMES")
	for _, p := range verbose.Containers {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			p.ID, p.Image, p.Command, p.LocalVolumes, p.Size, p.RunningFor, p.Status, p.Names)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	fmt.Fprint(stdout, "\nLocal Volumes space usage:\n\n")
	w = newTabWriter(stdout)
	fmt.Fprintln(w, "VOLUME NAME\tLINKS\tSIZE")
	for _, p := range verbose.Volumes {
		fmt.Fprintf(w, "%s\t%s\t%s\n", p.Name, p.Links, p.Size)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "\nBuild cache usage: %s\n\n", humanSize(du.BuildCache.TotalSize))
	w = newTabWriter(stdout)
	fmt.Fprintln(w, "CACHE ID\tCACHE TYPE\tSIZE\tCREATED\tLAST USED\tUSAGE\tSHARED")
	for _, p := range verbose.BuildCache {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			p.ID, p.CacheType, p.Size, p.CreatedSince, p.LastUsedSince, p.UsageCount, p.Shared)
	}
	return w.Flush()
}

func newTabWriter(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 4, 8, 4, ' ', 0)
}

func executeTemplate(w io.Writer, tmpl *template.Template, data any) error {
	var b bytes.Buffer
	if err := tmpl.Execute(&b, data); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w, b.String())
	return err
}

func humanSize(size int64) string {
	return units.HumanSize(float64(size))
}

// humanReclaimable renders the reclaimable space the way Docker does: as a share of the total, when
// there is a total to compare it against.
func humanReclaimable(reclaimable, totalSize int64) string {
	if totalSize > 0 {
		return fmt.Sprintf("%s (%v%%)", humanSize(reclaimable), (reclaimable*100)/totalSize)
	}
	return humanSize(reclaimable)
}

// displayID shortens an identifier for the table output only. A custom format is meant to be
// consumed by something else, and the full identifier is what makes the resource addressable.
func displayID(id string, trunc bool) string {
	if !trunc {
		return id
	}
	return truncateID(id)
}

// truncateID shortens an identifier for display, dropping the digest algorithm when there is one.
func truncateID(id string) string {
	if _, hex, ok := strings.Cut(id, ":"); ok {
		id = hex
	}
	return idgen.TruncateID(id)
}
