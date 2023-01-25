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
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/cmd/network"
	"github.com/containerd/nerdctl/pkg/netutil"

	"github.com/spf13/cobra"
)

func newNetworkRmCommand() *cobra.Command {
	networkRmCommand := &cobra.Command{
		Use:               "rm [flags] NETWORK [NETWORK, ...]",
		Aliases:           []string{"remove"},
		Short:             "Remove one or more networks",
		Long:              "NOTE: network in use is deleted without caution",
		Args:              cobra.MinimumNArgs(1),
		RunE:              networkRmAction,
		ValidArgsFunction: networkRmShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	return networkRmCommand
}

func networkRmAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}

	options := types.NetworkRemoveOptions{
		GOptions: globalOptions,
		Networks: args,
		Stdout:   cmd.OutOrStdout(),
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return network.Remove(ctx, client, options)
}

func networkRmShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show network names, including "bridge"
	exclude := []string{netutil.DefaultNetworkName, "host", "none"}
	return shellCompleteNetworkNames(cmd, exclude)
}
