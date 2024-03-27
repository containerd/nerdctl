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
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/mountutil/volumestore"
	"github.com/spf13/cobra"
)

func newNamespaceCommand() *cobra.Command {
	namespaceCommand := &cobra.Command{
		Annotations:   map[string]string{Category: Management},
		Use:           "namespace",
		Aliases:       []string{"ns"},
		Short:         "Manage containerd namespaces",
		Long:          "Unrelated to Linux namespaces and Kubernetes namespaces",
		RunE:          unknownSubcommandAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	namespaceCommand.AddCommand(newNamespaceLsCommand())
	namespaceCommand.AddCommand(newNamespaceRmCommand())
	namespaceCommand.AddCommand(newNamespaceCreateCommand())
	namespaceCommand.AddCommand(newNamespacelabelUpdateCommand())
	namespaceCommand.AddCommand(newNamespaceInspectCommand())
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
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
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
	dataStore, err := clientutil.DataStore(globalOptions.DataRoot, globalOptions.Address)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 4, 8, 4, ' ', 0)
	// no "NETWORKS", because networks are global objects
	fmt.Fprintln(w, "NAME\tCONTAINERS\tIMAGES\tVOLUMES\tLABELS")
	for _, ns := range nsList {
		ctx = namespaces.WithNamespace(ctx, ns)
		var numContainers, numImages, numVolumes int
		var labelStrings []string

		containers, err := client.Containers(ctx)
		if err != nil {
			log.L.Warn(err)
		}
		numContainers = len(containers)

		images, err := client.ImageService().List(ctx)
		if err != nil {
			log.L.Warn(err)
		}
		numImages = len(images)

		volStore, err := volumestore.Path(dataStore, ns)
		if err != nil {
			log.L.Warn(err)
		} else {
			volEnts, err := os.ReadDir(volStore)
			if err != nil {
				if !os.IsNotExist(err) {
					log.L.Warn(err)
				}
			}
			numVolumes = len(volEnts)
		}

		labels, err := client.NamespaceService().Labels(ctx, ns)
		if err != nil {
			return err
		}
		for k, v := range labels {
			labelStrings = append(labelStrings, strings.Join([]string{k, v}, "="))
		}
		sort.Strings(labelStrings)
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%v\t\n", ns, numContainers, numImages, numVolumes, strings.Join(labelStrings, ","))
	}
	return w.Flush()
}
