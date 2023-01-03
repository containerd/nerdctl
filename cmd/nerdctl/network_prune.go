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
	"context"
	"fmt"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/netutil"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var networkDriversToKeep = []string{"host", "none", DefaultNetworkDriver}

func newNetworkPruneCommand() *cobra.Command {
	networkPruneCommand := &cobra.Command{
		Use:           "prune [flags]",
		Short:         "Remove all unused networks",
		Args:          cobra.NoArgs,
		RunE:          networkPruneAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	networkPruneCommand.Flags().BoolP("force", "f", false, "Do not prompt for confirmation")
	return networkPruneCommand
}

func networkPruneAction(cmd *cobra.Command, _ []string) error {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}

	if !force {
		var confirm string
		msg := "This will remove all custom networks not used by at least one container."
		msg += "\nAre you sure you want to continue? [y/N] "

		fmt.Fprintf(cmd.OutOrStdout(), "WARNING! %s", msg)
		fmt.Fscanf(cmd.InOrStdin(), "%s", &confirm)

		if strings.ToLower(confirm) != "y" {
			return nil
		}
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return networkPrune(ctx, cmd, client, globalOptions)
}

func networkPrune(ctx context.Context, cmd *cobra.Command, client *containerd.Client, globalOptions *types.GlobalCommandOptions) error {
	e, err := netutil.NewCNIEnv(globalOptions.CNIPath, globalOptions.CNINetConfPath)
	if err != nil {
		return err
	}

	usedNetworks, err := netutil.UsedNetworks(ctx, client)
	if err != nil {
		return err
	}

	networkConfigs, err := e.NetworkList()
	if err != nil {
		return err
	}

	var removedNetworks []string // nolint: prealloc
	for _, net := range networkConfigs {
		if strutil.InStringSlice(networkDriversToKeep, net.Name) {
			continue
		}
		if net.NerdctlID == nil || net.File == "" {
			continue
		}
		if _, ok := usedNetworks[net.Name]; ok {
			continue
		}
		if err := e.RemoveNetwork(net); err != nil {
			logrus.WithError(err).Errorf("failed to remove network %s", net.Name)
			continue
		}
		removedNetworks = append(removedNetworks, net.Name)
	}

	if len(removedNetworks) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Deleted Networks:")
		for _, name := range removedNetworks {
			fmt.Fprintln(cmd.OutOrStdout(), name)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "")
	}
	return nil
}
