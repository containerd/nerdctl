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
	"strconv"
	"text/tabwriter"
	"text/template"

	"github.com/containerd/nerdctl/pkg/netutil"
	"github.com/docker/cli/templates"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

var networkLsCommand = &cli.Command{
	Name:    "ls",
	Aliases: []string{"list"},
	Usage:   "List networks",
	Action:  networkLsAction,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "quiet",
			Aliases: []string{"q"},
			Usage:   "Only display network IDs",
		},
		&cli.StringFlag{
			Name: "format",
			// Alias "-f" is reserved for "--filter"
			Usage: "Format the output using the given Go template, e.g, '{{json .}}'",
		},
	},
}

type networkPrintable struct {
	ID     string // empty for non-nerdctl networks
	Name   string
	Labels string
	// TODO: "CreatedAt", "Driver", "IPv6", "Internal", "Scope"
	file string `json:"-"`
}

func networkLsAction(clicontext *cli.Context) error {
	quiet := clicontext.Bool("quiet")
	w := clicontext.App.Writer
	var tmpl *template.Template
	switch format := clicontext.String("format"); format {
	case "", "table":
		w = tabwriter.NewWriter(clicontext.App.Writer, 4, 8, 4, ' ', 0)
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
		tmpl, err = templates.Parse(format)
		if err != nil {
			return err
		}
	}

	e := &netutil.CNIEnv{
		Path:        clicontext.String("cni-path"),
		NetconfPath: clicontext.String("cni-netconfpath"),
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
