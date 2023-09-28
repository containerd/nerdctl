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
	"errors"
	"fmt"
	"io"
	"text/tabwriter"
	"text/template"

	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/cmd/container"
	"github.com/containerd/nerdctl/pkg/formatter"

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

func processOptions(cmd *cobra.Command) (types.ContainerListOptions, FormattingAndPrintingOptions, error) {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return types.ContainerListOptions{}, FormattingAndPrintingOptions{}, err
	}
	all, err := cmd.Flags().GetBool("all")
	if err != nil {
		return types.ContainerListOptions{}, FormattingAndPrintingOptions{}, err
	}
	latest, err := cmd.Flags().GetBool("latest")
	if err != nil {
		return types.ContainerListOptions{}, FormattingAndPrintingOptions{}, err
	}

	lastN, err := cmd.Flags().GetInt("last")
	if err != nil {
		return types.ContainerListOptions{}, FormattingAndPrintingOptions{}, err
	}
	if lastN == -1 && latest {
		lastN = 1
	}

	filters, err := cmd.Flags().GetStringSlice("filter")
	if err != nil {
		return types.ContainerListOptions{}, FormattingAndPrintingOptions{}, err
	}

	noTrunc, err := cmd.Flags().GetBool("no-trunc")
	if err != nil {
		return types.ContainerListOptions{}, FormattingAndPrintingOptions{}, err
	}
	trunc := !noTrunc

	quiet, err := cmd.Flags().GetBool("quiet")
	if err != nil {
		return types.ContainerListOptions{}, FormattingAndPrintingOptions{}, err
	}
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return types.ContainerListOptions{}, FormattingAndPrintingOptions{}, err
	}

	size := false
	if !quiet {
		size, err = cmd.Flags().GetBool("size")
		if err != nil {
			return types.ContainerListOptions{}, FormattingAndPrintingOptions{}, err
		}
	}

	return types.ContainerListOptions{
			GOptions: globalOptions,
			All:      all,
			LastN:    lastN,
			Truncate: trunc,
			Size:     size || (format == "wide" && !quiet),
			Filters:  filters,
		}, FormattingAndPrintingOptions{
			Stdout: cmd.OutOrStdout(),
			Quiet:  quiet,
			Format: format,
			Size:   size,
		}, nil
}

func psAction(cmd *cobra.Command, args []string) error {
	clOpts, fpOpts, err := processOptions(cmd)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), clOpts.GOptions.Namespace, clOpts.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	containers, err := container.List(ctx, client, clOpts)
	if err != nil {
		return err
	}

	return formatAndPrintContainerInfo(containers, fpOpts)
}

// FormattingAndPrintingOptions specifies options for formatting and printing of `nerdctl (container) list`.
type FormattingAndPrintingOptions struct {
	Stdout io.Writer
	// Only display container IDs.
	Quiet bool
	// Format the output using the given Go template (e.g., '{{json .}}', 'table', 'wide').
	Format string
	// Display total file sizes.
	Size bool
}

func formatAndPrintContainerInfo(containers []container.ListItem, options FormattingAndPrintingOptions) error {
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
		if tmpl != nil {
			var b bytes.Buffer
			if err := tmpl.Execute(&b, c); err != nil {
				return err
			}
			if _, err := fmt.Fprintln(w, b.String()); err != nil {
				return err
			}
		} else if options.Quiet {
			if _, err := fmt.Fprintln(w, c.ID); err != nil {
				return err
			}
		} else {
			var name string
			if len(c.Names) > 0 {
				name = c.Names[0]
			}
			format := "%s\t%s\t%s\t%s\t%s\t%s\t%s"
			args := []interface{}{
				c.ID,
				c.Image,
				c.Command,
				formatter.TimeSinceInHuman(c.CreatedAt),
				c.Status,
				c.Ports,
				name,
			}
			if wide {
				format += "\t%s\t%s\t%s\n"
				args = append(args, c.Runtime, c.Platform, c.Size)
			} else if options.Size {
				format += "\t%s\n"
				args = append(args, c.Size)
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
