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

package namespace

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/mountutil/volumestore"
)

func Command() *cobra.Command {
	cmd := &cobra.Command{
		Annotations:   map[string]string{helpers.Category: helpers.Management},
		Use:           "namespace",
		Aliases:       []string{"ns"},
		Short:         "Manage containerd namespaces",
		Long:          "Unrelated to Linux namespaces and Kubernetes namespaces",
		RunE:          helpers.UnknownSubcommandAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(listCommand())
	cmd.AddCommand(removeCommand())
	cmd.AddCommand(createCommand())
	cmd.AddCommand(updateCommand())
	cmd.AddCommand(inspectCommand())
	return cmd
}

func listCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "ls",
		Aliases:       []string{"list"},
		Short:         "List containerd namespaces",
		RunE:          listAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.Flags().BoolP("quiet", "q", false, "Only display names")
	return cmd
}

func listAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
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

		volStore, err := volumestore.New(dataStore, ns)
		if err != nil {
			log.L.Warn(err)
		} else {
			numVolumes, err = volStore.Count()
			if err != nil {
				log.L.Warn(err)
			}
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
