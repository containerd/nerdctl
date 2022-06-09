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

	"github.com/containerd/nerdctl/pkg/lockutil"
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
	cniPath, err := cmd.Flags().GetString("cni-path")
	if err != nil {
		return err
	}
	cniNetconfpath, err := cmd.Flags().GetString("cni-netconfpath")
	if err != nil {
		return err
	}
	e, err := netutil.NewCNIEnv(cniPath, cniNetconfpath)
	if err != nil {
		return err
	}
	fn := func() error {
		netMap := e.NetworkMap()

		for _, name := range args {
			if name == "host" || name == "none" {
				return fmt.Errorf("pseudo network %q cannot be removed", name)
			}
			l, ok := netMap[name]
			if !ok {
				return fmt.Errorf("no such network: %s", name)
			}
			if l.NerdctlID == nil {
				return fmt.Errorf("%s is managed outside nerdctl and cannot be removed", name)
			}
			if l.File == "" {
				return fmt.Errorf("%s is a pre-defined network and cannot be removed", name)
			}
			if err := os.RemoveAll(l.File); err != nil {
				return err
			}
			// Remove the bridge network interface on the host.
			if l.Plugins[0].Network.Type == "bridge" {
				netIf := netutil.GetBridgeName(*l.NerdctlID)
				removeBridgeNetworkInterface(netIf)
			}
			fmt.Fprintln(cmd.OutOrStdout(), name)
		}
		return nil
	}
	return lockutil.WithDirLock(cniNetconfpath, fn)
}

func networkRmShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show network names, including "bridge"
	exclude := []string{netutil.DefaultNetworkName, "host", "none"}
	return shellCompleteNetworkNames(cmd, exclude)
}
