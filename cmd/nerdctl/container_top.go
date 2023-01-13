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
	"errors"
	"fmt"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/cmd/container"
	"github.com/containerd/nerdctl/pkg/infoutil"
	"github.com/containerd/nerdctl/pkg/rootlessutil"

	"github.com/spf13/cobra"
)

func newTopCommand() *cobra.Command {
	var topCommand = &cobra.Command{
		Use:               "top CONTAINER [ps OPTIONS]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Display the running processes of a container",
		RunE:              topAction,
		ValidArgsFunction: topShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	topCommand.Flags().SetInterspersed(false)
	return topCommand
}

func topAction(cmd *cobra.Command, args []string) error {
	// NOTE: rootless container does not rely on cgroupv1.
	// more details about possible ways to resolve this concern: #223
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	if rootlessutil.IsRootless() && infoutil.CgroupsVersion() == "1" {
		return fmt.Errorf("top requires cgroup v2 for rootless containers, see https://rootlesscontaine.rs/getting-started/common/cgroup2/")
	}

	if globalOptions.CgroupManager == "none" {
		return errors.New("cgroup manager must not be \"none\"")
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()
	return container.Top(ctx, client, args, types.ContainerTopOptions{
		Stdout:   cmd.OutOrStdout(),
		GOptions: globalOptions,
	})

}

func topShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show running container names
	statusFilterFn := func(st containerd.ProcessStatus) bool {
		return st == containerd.Running
	}
	return shellCompleteContainerNames(cmd, statusFilterFn)
}
