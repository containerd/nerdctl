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
	"io/ioutil"
	"os"
	"text/tabwriter"

	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/nerdctl/pkg/mountutil/volumestore"
	"github.com/sirupsen/logrus"
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

	dataStore, err := getDataStore(clicontext)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(clicontext.App.Writer, 4, 8, 4, ' ', 0)
	// no "NETWORKS", because networks are global objects
	fmt.Fprintln(w, "NAME\tCONTAINERS\tIMAGES\tVOLUMES")
	for _, ns := range nsList {
		ctx = namespaces.WithNamespace(ctx, ns)
		var numContainers, numImages, numVolumes int

		containers, err := client.Containers(ctx)
		if err != nil {
			logrus.Warn(err)
		}
		numContainers = len(containers)

		images, err := client.ImageService().List(ctx)
		if err != nil {
			logrus.Warn(err)
		}
		numImages = len(images)

		volStore, err := volumestore.Path(dataStore, ns)
		if err != nil {
			logrus.Warn(err)
		} else {
			volEnts, err := ioutil.ReadDir(volStore)
			if err != nil {
				if !os.IsNotExist(err) {
					logrus.Warn(err)
				}
			}
			numVolumes = len(volEnts)
		}

		fmt.Fprintf(w, "%s\t%d\t%d\t%d\n", ns, numContainers, numImages, numVolumes)
	}
	return w.Flush()
}
