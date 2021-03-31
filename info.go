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
	"fmt"
	"strings"
	"text/template"

	"github.com/containerd/nerdctl/pkg/infoutil"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/docker/cli/templates"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var infoCommand = &cli.Command{
	Name:   "info",
	Usage:  "Display system-wide information",
	Action: infoAction,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "format",
			Aliases: []string{"f"},
			Usage:   "Format the output using the given Go template, e.g, '{{json .}}'",
		},
	},
}

func infoAction(clicontext *cli.Context) error {
	w := clicontext.App.Writer
	var (
		tmpl *template.Template
		err  error
	)
	if format := clicontext.String("format"); format != "" {
		tmpl, err = templates.Parse(format)
		if err != nil {
			return err
		}
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	info, err := infoutil.Info(ctx, client, clicontext.String("snapshotter"))
	if err != nil {
		return err
	}

	if tmpl != nil {
		if err := tmpl.Execute(w, info); err != nil {
			return err
		}
		_, err = fmt.Fprintf(w, "\n")
		return err
	}

	fmt.Fprintf(w, "Client:\n")
	fmt.Fprintf(w, " Namespace:\t%s\n", clicontext.String("namespace"))
	fmt.Fprintf(w, " Debug Mode:\t%v\n", clicontext.Bool("debug"))
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "Server:\n")
	fmt.Fprintf(w, " Server Version: %s\n", info.ServerVersion)
	// Storage Driver is not really Server concept for nerdctl, but mimics `docker info` output
	fmt.Fprintf(w, " Storage Driver: %s\n", info.Driver)
	fmt.Fprintf(w, " Logging Driver: %s\n", info.LoggingDriver)
	fmt.Fprintf(w, " Cgroup Driver: %s\n", info.CgroupDriver)
	fmt.Fprintf(w, " Cgroup Version: %s\n", info.CgroupVersion)
	fmt.Fprintf(w, " Plugins:\n")
	fmt.Fprintf(w, "  Log: %s\n", strings.Join(info.Plugins.Log, " "))
	fmt.Fprintf(w, "  Storage: %s\n", strings.Join(info.Plugins.Storage, " "))
	fmt.Fprintf(w, " Security Options:\n")
	for _, s := range info.SecurityOptions {
		m, err := strutil.ParseCSVMap(s)
		if err != nil {
			logrus.WithError(err).Warnf("unparsable security option %q", s)
			continue
		}
		name := m["name"]
		if name == "" {
			logrus.Warnf("unparsable security option %q", s)
			continue
		}
		fmt.Fprintf(w, "  %s\n", name)
		for k, v := range m {
			if k == "name" {
				continue
			}
			fmt.Fprintf(w, "   %s: %s\n", strings.Title(k), v)
		}
	}
	fmt.Fprintf(w, " Kernel Version: %s\n", info.KernelVersion)
	fmt.Fprintf(w, " Operating System: %s\n", info.OperatingSystem)
	fmt.Fprintf(w, " OSType: %s\n", info.OSType)
	fmt.Fprintf(w, " Architecture: %s\n", info.Architecture)
	fmt.Fprintf(w, " Name: %s\n", info.Name)
	fmt.Fprintf(w, " ID: %s\n", info.ID)
	return nil
}
