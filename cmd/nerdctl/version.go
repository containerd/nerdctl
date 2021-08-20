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
	"text/template"

	"github.com/containerd/nerdctl/pkg/infoutil"
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/docker/cli/templates"
	"github.com/urfave/cli/v2"
)

var versionCommand = &cli.Command{
	Name:   "version",
	Usage:  "Show the nerdctl version information",
	Action: versionAction,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "format",
			Aliases: []string{"f"},
			Usage:   "Format the output using the given Go template, e.g, '{{json .}}'",
		},
	},
}

func versionAction(clicontext *cli.Context) error {
	w := clicontext.App.Writer
	var tmpl *template.Template
	if format := clicontext.String("format"); format != "" {
		var err error
		tmpl, err = templates.Parse(format)
		if err != nil {
			return err
		}
	}

	v, vErr := versionInfo(clicontext)
	if tmpl != nil {
		var b bytes.Buffer
		if err := tmpl.Execute(&b, v); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, b.String()+"\n"); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(w, "Client:\n")
		fmt.Fprintf(w, " Version:\t%s\n", v.Client.Version)
		fmt.Fprintf(w, " Git commit:\t%s\n", v.Client.GitCommit)
		if v.Server != nil {
			fmt.Fprintf(w, "\n")
			fmt.Fprintf(w, "Server:\n")
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
func versionInfo(clicontext *cli.Context) (dockercompat.VersionInfo, error) {
	v := dockercompat.VersionInfo{
		Client: infoutil.ClientVersion(),
	}
	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return v, err
	}
	defer cancel()
	v.Server, err = infoutil.ServerVersion(ctx, client)
	return v, err
}
