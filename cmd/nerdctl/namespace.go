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
	"os"
	"text/tabwriter"

	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/nerdctl/pkg/mountutil/volumestore"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newNamespaceCommand() *cobra.Command {
	namespaceCommand := &cobra.Command{
		Annotations:   map[string]string{Category: Management},
		Use:           "namespace",
		Short:         "Manage containerd namespaces",
		Long:          "Unrelated to Linux namespaces and Kubernetes namespaces",
		RunE:          unknownSubcommandAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	namespaceCommand.AddCommand(newNamespaceLsCommand())
	return namespaceCommand
}

func newNamespaceLsCommand() *cobra.Command {
	namespaceLsCommand := &cobra.Command{
		Use:           "ls",
		Aliases:       []string{"list"},
		Short:         "List containerd namespaces",
		RunE:          namespaceLsAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	namespaceLsCommand.Flags().BoolP("quiet", "q", false, "Only display names")
	return namespaceLsCommand
}

func namespaceLsAction(cmd *cobra.Command, args []string) error {
	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	nsService := client.NamespaceService()
	nsList, err := nsService.List(ctx)
	if err != nil {
		return err
	}
	quiet, err := cmd.Flags().GetBool("quiet")
	if err != nil {
		return err
	}
	if quiet {
		for _, ns := range nsList {
			fmt.Fprintln(cmd.OutOrStdout(), ns)
		}
		return nil
	}

	dataStore, err := getDataStore(cmd)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 4, 8, 4, ' ', 0)
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
			volEnts, err := os.ReadDir(volStore)
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
