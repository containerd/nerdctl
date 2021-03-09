/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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
	"text/tabwriter"

	"github.com/urfave/cli/v2"
)

var namespaceCommand = &cli.Command{
	Name:        "namespace",
	Usage:       "Manage containerd namespaces",
	Description: "Unrelated to Linux namespaces and Kubernetes namespaces",
	Category:    CategoryManagement,
	Subcommands: []*cli.Command{
		namespaceLsCommand,
	},
}

var namespaceLsCommand = &cli.Command{
	Name:    "ls",
	Aliases: []string{"list"},
	Usage:   "List containerd namespaces",
	Action:  namespaceLsAction,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "quiet",
			Aliases: []string{"q"},
			Usage:   "Only display namespace names",
		},
	},
}

func namespaceLsAction(clicontext *cli.Context) error {
	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	nsService := client.NamespaceService()
	nsList, err := nsService.List(ctx)
	if err != nil {
		return err
	}
	if clicontext.Bool("q") {
		for _, ns := range nsList {
			fmt.Fprintln(clicontext.App.Writer, ns)
		}
		return nil
	}

	w := tabwriter.NewWriter(clicontext.App.Writer, 4, 8, 4, ' ', 0)
	fmt.Fprintln(w, "NAME")
	for _, ns := range nsList {
		fmt.Fprintf(w, "%ss\n", ns)
	}
	return w.Flush()
}
