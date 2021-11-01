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
	"strconv"
	"text/tabwriter"
	"text/template"

	"github.com/containerd/nerdctl/pkg/netutil"

	"github.com/spf13/cobra"
)

func newNetworkLsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "ls",
		Aliases:       []string{"list"},
		Short:         "List networks",
		Args:          cobra.NoArgs,
		RunE:          networkLsAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.Flags().BoolP("quiet", "q", false, "Only display network IDs")
	// Alias "-f" is reserved for "--filter"
	cmd.Flags().String("format", "", "Format the output using the given Go template, e.g, '{{json .}}'")
	cmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json"}, cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}

type networkPrintable struct {
	ID     string // empty for non-nerdctl networks
	Name   string
	Labels string
	// TODO: "CreatedAt", "Driver", "IPv6", "Internal", "Scope"
	file string `json:"-"`
}

func networkLsAction(cmd *cobra.Command, args []string) error {
	quiet, err := cmd.Flags().GetBool("quiet")
	if err != nil {
		return err
	}
	w := cmd.OutOrStdout()
	var tmpl *template.Template
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	switch format {
	case "", "table":
		w = tabwriter.NewWriter(cmd.OutOrStdout(), 4, 8, 4, ' ', 0)
		if !quiet {
			fmt.Fprintln(w, "NETWORK ID\tNAME\tFILE")
		}
	case "raw":
		return errors.New("unsupported format: \"raw\"")
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

	cniPath, err := cmd.Flags().GetString("cni-path")
	if err != nil {
		return err
	}
	cniNetconfpath, err := cmd.Flags().GetString("cni-netconfpath")
	if err != nil {
		return err
	}
	e := &netutil.CNIEnv{
		Path:        cniPath,
		NetconfPath: cniNetconfpath,
	}
	ll, err := netutil.ConfigLists(e)
	if err != nil {
		return err
	}
	pp := make([]networkPrintable, len(ll))
	for i, l := range ll {
		p := networkPrintable{
			Name: l.Name,
			file: l.File,
		}
		if l.NerdctlID != nil {
			p.ID = strconv.Itoa(*l.NerdctlID)
		}
		if l.NerdctlLabels != nil {
			p.Labels = formatLabels(*l.NerdctlLabels)
		}
		pp[i] = p
	}

	// append pseudo networks
	pp = append(pp, []networkPrintable{
		{
			Name: "host",
		},
		{
			Name: "none",
		},
	}...)

	for _, p := range pp {
		if tmpl != nil {
			var b bytes.Buffer
			if err := tmpl.Execute(&b, p); err != nil {
				return err
			}
			if _, err = fmt.Fprintf(w, b.String()+"\n"); err != nil {
				return err
			}
		} else if quiet {
			if p.ID != "" {
				fmt.Fprintln(w, p.ID)
			}
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\n", p.ID, p.Name, p.file)
		}
	}
	if f, ok := w.(Flusher); ok {
		return f.Flush()
	}
	return nil
}
