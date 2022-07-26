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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/netutil"
	"github.com/containerd/nerdctl/pkg/netutil/nettype"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

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

func networkPruneAction(cmd *cobra.Command, args []string) error {
	cniPath, err := cmd.Flags().GetString("cni-path")
	if err != nil {
		return err
	}
	cniNetconfpath, err := cmd.Flags().GetString("cni-netconfpath")
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

	e, err := netutil.NewCNIEnv(cniPath, cniNetconfpath)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	containers, err := client.Containers(ctx)
	if err != nil {
		return err
	}

	usedNetworks, err := usedNetworks(ctx, containers)
	if err != nil {
		return err
	}

	var removedNetworks []string // nolint: prealloc
	for _, net := range e.Networks {
		if net.Name == "host" || net.Name == "none" {
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
	}
	return nil
}

func usedNetworks(ctx context.Context, containers []containerd.Container) (map[string]struct{}, error) {
	used := make(map[string]struct{})
	for _, c := range containers {
		task, err := c.Task(ctx, nil)
		if err != nil {
			return nil, err
		}
		status, err := task.Status(ctx)
		if err != nil {
			return nil, err
		}
		switch status.Status {
		case containerd.Paused, containerd.Running:
		default:
			continue
		}
		l, err := c.Labels(ctx)
		if err != nil {
			return nil, err
		}
		networkJSON, ok := l[labels.Networks]
		if !ok {
			continue
		}
		var networks []string
		if err := json.Unmarshal([]byte(networkJSON), &networks); err != nil {
			return nil, err
		}
		netType, err := nettype.Detect(networks)
		if err != nil {
			return nil, err
		}
		if netType != nettype.CNI {
			continue
		}
		for _, n := range networks {
			used[n] = struct{}{}
		}
	}
	return used, nil
}
