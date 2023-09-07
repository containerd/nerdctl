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
	"fmt"
	"io"
	"os"
	"text/template"

	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/infoutil"
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/spf13/cobra"
)

func newVersionCommand() *cobra.Command {
	var versionCommand = &cobra.Command{
		Use:           "version",
		Args:          cobra.NoArgs,
		Short:         "Show the nerdctl version information",
		RunE:          versionAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	versionCommand.Flags().StringP("format", "f", "", "Format the output using the given Go template, e.g, '{{json .}}'")
	versionCommand.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json"}, cobra.ShellCompDirectiveNoFileComp
	})
	return versionCommand
}

func versionAction(cmd *cobra.Command, args []string) error {
	var w io.Writer = os.Stdout
	var tmpl *template.Template
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	if format != "" {
		var err error
		tmpl, err = formatter.ParseTemplate(format)
		if err != nil {
			return err
		}
	}

	v, vErr := versionInfo(cmd, globalOptions)
	if tmpl != nil {
		var b bytes.Buffer
		if err := tmpl.Execute(&b, v); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, b.String()); err != nil {
			return err
		}
	} else {
		fmt.Fprintln(w, "Client:")
		fmt.Fprintf(w, " Version:\t%s\n", v.Client.Version)
		fmt.Fprintf(w, " OS/Arch:\t%s/%s\n", v.Client.Os, v.Client.Arch)
		fmt.Fprintf(w, " Git commit:\t%s\n", v.Client.GitCommit)
		for _, compo := range v.Client.Components {
			fmt.Fprintf(w, " %s:\n", compo.Name)
			fmt.Fprintf(w, "  Version:\t%s\n", compo.Version)
			for detailK, detailV := range compo.Details {
				fmt.Fprintf(w, "  %s:\t%s\n", detailK, detailV)
			}
		}
		if v.Server != nil {
			fmt.Fprintln(w)
			fmt.Fprintln(w, "Server:")
			for _, compo := range v.Server.Components {
				fmt.Fprintf(w, " %s:\n", compo.Name)
				fmt.Fprintf(w, "  Version:\t%s\n", compo.Version)
				for detailK, detailV := range compo.Details {
					fmt.Fprintf(w, "  %s:\t%s\n", detailK, detailV)
				}
			}
		}
	}
	return vErr
}

// versionInfo may return partial VersionInfo on error
func versionInfo(cmd *cobra.Command, globalOptions types.GlobalCommandOptions) (dockercompat.VersionInfo, error) {

	v := dockercompat.VersionInfo{
		Client: infoutil.ClientVersion(),
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return v, err
	}
	defer cancel()
	v.Server, err = infoutil.ServerVersion(ctx, client)
	return v, err
}
