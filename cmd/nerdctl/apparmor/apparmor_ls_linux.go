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

package apparmor

import (
	"bytes"
	"errors"
	"fmt"
	"text/tabwriter"
	"text/template"

	"github.com/containerd/nerdctl/cmd/nerdctl/utils/fmtutil"
	"github.com/containerd/nerdctl/pkg/apparmorutil"
	"github.com/spf13/cobra"
)

func newApparmorLsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "ls",
		Aliases:       []string{"list"},
		Short:         "List the loaded AppArmor profiles",
		Args:          cobra.NoArgs,
		RunE:          LsAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.Flags().BoolP("quiet", "q", false, "Only display profile names")
	// Alias "-f" is reserved for "--filter"
	cmd.Flags().String("format", "", "Format the output using the given go template")
	cmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json", "table", "wide"}, cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}

func LsAction(cmd *cobra.Command, args []string) error {
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
	case "", "table", "wide":
		w = tabwriter.NewWriter(cmd.OutOrStdout(), 4, 8, 4, ' ', 0)
		if !quiet {
			fmt.Fprintln(w, "NAME\tMODE")
		}
	case "raw":
		return errors.New("unsupported format: \"raw\"")
	default:
		if quiet {
			return errors.New("format and quiet must not be specified together")
		}
		var err error
		tmpl, err = fmtutil.ParseTemplate(format)
		if err != nil {
			return err
		}
	}

	profiles, err := apparmorutil.Profiles()
	if err != nil {
		return err
	}

	for _, f := range profiles {
		if tmpl != nil {
			var b bytes.Buffer
			if err := tmpl.Execute(&b, f); err != nil {
				return err
			}
			if _, err = fmt.Fprintf(w, b.String()+"\n"); err != nil {
				return err
			}
		} else if quiet {
			fmt.Fprintln(w, f.Name)
		} else {
			fmt.Fprintf(w, "%s\t%s\n", f.Name, f.Mode)
		}
	}
	if f, ok := w.(fmtutil.Flusher); ok {
		return f.Flush()
	}
	return nil
}
