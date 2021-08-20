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
	"text/tabwriter"
	"text/template"

	"github.com/containerd/nerdctl/pkg/inspecttypes/native"
	"github.com/docker/cli/templates"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

var volumeLsCommand = &cli.Command{
	Name:    "ls",
	Aliases: []string{"list"},
	Usage:   "List volumes",
	Action:  volumeLsAction,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "quiet",
			Aliases: []string{"q"},
			Usage:   "Only display volume names",
		},
		&cli.StringFlag{
			Name: "format",
			// Alias "-f" is reserved for "--filter"
			Usage: "Format the output using the given Go template, e.g, '{{json .}}'",
		},
	},
}

type volumePrintable struct {
	Driver     string
	Labels     string
	Mountpoint string
	Name       string
	Scope      string
	// TODO: "Links", "Size"
}

func volumeLsAction(clicontext *cli.Context) error {
	quiet := clicontext.Bool("quiet")
	w := clicontext.App.Writer
	var tmpl *template.Template
	switch format := clicontext.String("format"); format {
	case "", "table":
		w = tabwriter.NewWriter(clicontext.App.Writer, 4, 8, 4, ' ', 0)
		if !quiet {
			fmt.Fprintln(w, "VOLUME NAME\tDIRECTORY")
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

	vols, err := getVolumes(clicontext)
	if err != nil {
		return err
	}

	for _, v := range vols {
		p := volumePrintable{
			Driver:     "local",
			Labels:     "",
			Mountpoint: v.Mountpoint,
			Name:       v.Name,
			Scope:      "local",
		}
		if v.Labels != nil {
			p.Labels = formatLabels(*v.Labels)
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
			fmt.Fprintln(w, p.Name)
		} else {
			fmt.Fprintf(w, "%s\t%s\n", p.Name, p.Mountpoint)
		}
	}
	if f, ok := w.(Flusher); ok {
		return f.Flush()
	}
	return nil
}

func getVolumes(clicontext *cli.Context) (map[string]native.Volume, error) {
	volStore, err := getVolumeStore(clicontext)
	if err != nil {
		return nil, err
	}
	return volStore.List()
}
