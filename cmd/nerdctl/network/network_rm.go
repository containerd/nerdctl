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

package network

import (
	"context"
	"fmt"

	nerdClient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/common"
	"github.com/containerd/nerdctl/pkg/idutil/netwalker"
	"github.com/containerd/nerdctl/pkg/netutil"
	"github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
)

func NewNetworkRmCommand() *cobra.Command {
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
	client, ctx, cancel, err := nerdClient.NewClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()
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

	usedNetworkInfo, err := netutil.UsedNetworks(ctx, client)
	if err != nil {
		return err
	}

	walker := netwalker.NetworkWalker{
		Client: e,
		OnFound: func(ctx context.Context, found netwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			if value, ok := usedNetworkInfo[found.Network.Name]; ok {
				return fmt.Errorf("network %q is in use by container %q", found.Req, value)
			}
			if found.Network.NerdctlID == nil {
				return fmt.Errorf("%s is managed outside nerdctl and cannot be removed", found.Req)
			}
			if found.Network.File == "" {
				return fmt.Errorf("%s is a pre-defined network and cannot be removed", found.Req)
			}
			if err := e.RemoveNetwork(found.Network); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), found.Req)
			return nil
		},
	}

	code := 0
	for _, name := range args {
		if name == "host" || name == "none" {
			code = 1
			logrus.Errorf("pseudo network %q cannot be removed", name)
			continue
		}

		n, err := walker.Walk(cmd.Context(), name)
		if err != nil {
			code = 1
			logrus.Error(err)
			continue

		} else if n == 0 {
			code = 1
			logrus.Errorf("No such network: %s", name)
			continue
		}
	}

	// compatible with docker
	// ExitCodeError is to allow the program to exit with status code 1 without outputting an error message.
	if code != 0 {
		return common.ExitCodeError{
			Code: code,
		}
	}
	return nil
}

func networkRmShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show network names, including "bridge"
	exclude := []string{netutil.DefaultNetworkName, "host", "none"}
	return completion.ShellCompleteNetworkNames(cmd, exclude)
}
