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

package container

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	containerd "github.com/containerd/containerd/v2/client"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/container"
	"github.com/containerd/nerdctl/v2/pkg/infoutil"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
)

func TopCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:               "top CONTAINER [ps OPTIONS]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Display the running processes of a container",
		RunE:              topAction,
		ValidArgsFunction: topShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	cmd.Flags().SetInterspersed(false)
	return cmd
}

func topAction(cmd *cobra.Command, args []string) error {
	// NOTE: rootless container does not rely on cgroupv1.
	// more details about possible ways to resolve this concern: #223
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
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

	containerID := args[0]
	var psArgs string
	if len(args) > 1 {
		// Join all remaining arguments as ps args
		psArgs = strings.Join(args[1:], " ")
	}

	return container.Top(ctx, client, []string{containerID}, types.ContainerTopOptions{
		Stdout:   cmd.OutOrStdout(),
		GOptions: globalOptions,
		PsArgs:   psArgs,
	})

}

func topShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show running container names
	statusFilterFn := func(st containerd.ProcessStatus) bool {
		return st == containerd.Running
	}
	return completion.ContainerNames(cmd, statusFilterFn)
}
