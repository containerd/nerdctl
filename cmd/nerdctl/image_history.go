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
	"strconv"
	"text/tabwriter"
	"text/template"
	"time"

	"github.com/docker/go-units"
	"github.com/opencontainers/image-spec/identity"
	"github.com/spf13/cobra"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/formatter"
	"github.com/containerd/nerdctl/v2/pkg/idutil/imagewalker"
	"github.com/containerd/nerdctl/v2/pkg/imgutil"
)

func newHistoryCommand() *cobra.Command {
	var historyCommand = &cobra.Command{
		Use:               "history [flags] IMAGE",
		Short:             "Show the history of an image",
		Args:              helpers.IsExactArgs(1),
		RunE:              historyAction,
		ValidArgsFunction: historyShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	addHistoryFlags(historyCommand)
	return historyCommand
}

func addHistoryFlags(cmd *cobra.Command) {
	cmd.Flags().StringP("format", "f", "", "Format the output using the given Go template, e.g, '{{json .}}'")
	cmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().BoolP("quiet", "q", false, "Only show numeric IDs")
	cmd.Flags().BoolP("human", "H", true, "Print sizes and dates in human readable format (default true)")
	cmd.Flags().Bool("no-trunc", false, "Don't truncate output")
}

type historyPrintable struct {
	creationTime *time.Time
	size         int64

	Snapshot     string
	CreatedAt    string
	CreatedSince string
	CreatedBy    string
	Size         string
	Comment      string
}

func historyAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	walker := &imagewalker.ImageWalker{
		Client: client,
		OnFound: func(ctx context.Context, found imagewalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			img := containerd.NewImage(client, found.Image)
			imageConfig, _, err := imgutil.ReadImageConfig(ctx, img)
			if err != nil {
				return fmt.Errorf("failed to ReadImageConfig: %w", err)
			}
			configHistories := imageConfig.History
			layerCounter := 0
			diffIDs, err := img.RootFS(ctx)
			if err != nil {
				return fmt.Errorf("failed to get diffIDS: %w", err)
			}
			var historys []historyPrintable
			for _, h := range configHistories {
				var size int64
				var snapshotName string
				if !h.EmptyLayer {
					if len(diffIDs) <= layerCounter {
						return fmt.Errorf("too many non-empty layers in History section")
					}
					diffIDs := diffIDs[0 : layerCounter+1]
					chainID := identity.ChainID(diffIDs).String()

					s := client.SnapshotService(globalOptions.Snapshotter)
					stat, err := s.Stat(ctx, chainID)
					if err != nil {
						return fmt.Errorf("failed to get stat: %w", err)
					}
					use, err := s.Usage(ctx, chainID)
					if err != nil {
						return fmt.Errorf("failed to get usage: %w", err)
					}
					size = use.Size
					snapshotName = stat.Name
					layerCounter++
				} else {
					size = 0
					snapshotName = "<missing>"
				}
				history := historyPrintable{
					creationTime: h.Created,
					size:         size,
					Snapshot:     snapshotName,
					CreatedBy:    h.CreatedBy,
					Comment:      h.Comment,
				}
				historys = append(historys, history)
			}
			err = printHistory(cmd, historys)
			if err != nil {
				return fmt.Errorf("failed printHistory: %w", err)
			}
			return nil
		},
	}

	return walker.WalkAll(ctx, args, true)
}

type historyPrinter struct {
	w                     io.Writer
	quiet, noTrunc, human bool
	tmpl                  *template.Template
}

func printHistory(cmd *cobra.Command, historys []historyPrintable) error {
	quiet, err := cmd.Flags().GetBool("quiet")
	if err != nil {
		return err
	}
	noTrunc, err := cmd.Flags().GetBool("no-trunc")
	if err != nil {
		return err
	}
	human, err := cmd.Flags().GetBool("human")
	if err != nil {
		return err
	}

	var w io.Writer
	w = os.Stdout

	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}

	var tmpl *template.Template
	switch format {
	case "", "table":
		w = tabwriter.NewWriter(w, 4, 8, 4, ' ', 0)
		if !quiet {
			fmt.Fprintln(w, "SNAPSHOT\tCREATED\tCREATED BY\tSIZE\tCOMMENT")
		}
	case "raw":
		return errors.New("unsupported format: \"raw\"")
	default:
		quiet = false
		var err error
		tmpl, err = formatter.ParseTemplate(format)
		if err != nil {
			return err
		}
	}

	printer := &historyPrinter{
		w:       w,
		quiet:   quiet,
		noTrunc: noTrunc,
		human:   human,
		tmpl:    tmpl,
	}

	for index := len(historys) - 1; index >= 0; index-- {
		if err := printer.printHistory(historys[index]); err != nil {
			log.L.Warn(err)
		}
	}

	if f, ok := w.(formatter.Flusher); ok {
		return f.Flush()
	}
	return nil
}

func (x *historyPrinter) printHistory(printable historyPrintable) error {
	// Truncate long values unless --no-trunc is passed
	if !x.noTrunc {
		if len(printable.CreatedBy) > 45 {
			printable.CreatedBy = printable.CreatedBy[0:44] + "…"
		}
		// Do not truncate snapshot id if quiet is being passed
		if !x.quiet && len(printable.Snapshot) > 45 {
			printable.Snapshot = printable.Snapshot[0:44] + "…"
		}
	}

	// Format date and size for display based on --human preference
	printable.CreatedAt = printable.creationTime.Local().Format(time.RFC3339)
	if x.human {
		printable.CreatedSince = formatter.TimeSinceInHuman(*printable.creationTime)
		printable.Size = units.HumanSize(float64(printable.size))
	} else {
		printable.CreatedSince = printable.CreatedAt
		printable.Size = strconv.FormatInt(printable.size, 10)
	}

	if x.tmpl != nil {
		var b bytes.Buffer
		if err := x.tmpl.Execute(&b, printable); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(x.w, b.String()); err != nil {
			return err
		}
	} else if x.quiet {
		if _, err := fmt.Fprintln(x.w, printable.Snapshot); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(x.w, "%s\t%s\t%s\t%s\t%s\n",
			printable.Snapshot,
			printable.CreatedSince,
			printable.CreatedBy,
			printable.Size,
			printable.Comment,
		); err != nil {
			return err
		}
	}
	return nil
}

func historyShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show image names
	return completion.ImageNames(cmd)
}
