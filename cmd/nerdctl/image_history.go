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
	"text/tabwriter"
	"text/template"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/pkg/progress"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/idutil/imagewalker"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/opencontainers/image-spec/identity"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newHistoryCommand() *cobra.Command {
	var historyCommand = &cobra.Command{
		Use:               "history [flags] IMAGE",
		Short:             "Show the history of an image",
		Args:              IsExactArgs(1),
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
	cmd.Flags().Bool("no-trunc", false, "Don't truncate output")
}

type historyPrintable struct {
	Snapshot     string
	CreatedSince string
	CreatedBy    string
	Size         string
	Comment      string
}

func historyAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := processRootCmdFlags(cmd)
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
				var size string
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
					size = progress.Bytes(use.Size).String()
					snapshotName = stat.Name
					layerCounter++
				} else {
					size = progress.Bytes(0).String()
					snapshotName = "<missing>"
				}
				history := historyPrintable{
					Snapshot:     snapshotName,
					CreatedSince: formatter.TimeSinceInHuman(*h.Created),
					CreatedBy:    h.CreatedBy,
					Size:         size,
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
	w              io.Writer
	quiet, noTrunc bool
	tmpl           *template.Template
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
		if quiet {
			return errors.New("format and quiet must not be specified together")
		}
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
		tmpl:    tmpl,
	}

	for index := len(historys) - 1; index >= 0; index-- {
		if err := printer.printHistory(historys[index]); err != nil {
			logrus.Warn(err)
		}
	}

	if f, ok := w.(formatter.Flusher); ok {
		return f.Flush()
	}
	return nil
}

func (x *historyPrinter) printHistory(p historyPrintable) error {
	if !x.noTrunc {
		if len(p.CreatedBy) > 45 {
			p.CreatedBy = p.CreatedBy[0:44] + "…"
		}
	}
	if x.tmpl != nil {
		var b bytes.Buffer
		if err := x.tmpl.Execute(&b, p); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(x.w, b.String()); err != nil {
			return err
		}
	} else if x.quiet {
		if _, err := fmt.Fprintln(x.w, p.Snapshot); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(x.w, "%s\t%s\t%s\t%s\t%s\n",
			p.Snapshot,
			p.CreatedSince,
			p.CreatedBy,
			p.Size,
			p.Comment,
		); err != nil {
			return err
		}
	}
	return nil
}

func historyShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show image names
	return shellCompleteImageNames(cmd)
}
