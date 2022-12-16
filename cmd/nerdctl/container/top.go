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

/*
   Portions from:
   - https://github.com/moby/moby/blob/v20.10.6/api/types/container/container_top.go
   - https://github.com/moby/moby/blob/v20.10.6/daemon/top_unix.go
   Copyright (C) The Moby authors.
   Licensed under the Apache License, Version 2.0
   NOTICE: https://github.com/moby/moby/blob/v20.10.6/NOTICE
*/

package container

import (
	"context"
	"errors"
	"fmt"
	"strings"

	ncclient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/infoutil"
	"github.com/containerd/nerdctl/pkg/rootlessutil"

	"github.com/spf13/cobra"
)

func NewTopCommand() *cobra.Command {
	var topCommand = &cobra.Command{
		Use:               "top CONTAINER [ps OPTIONS]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Display the running processes of a container",
		RunE:              topAction,
		ValidArgsFunction: completion.TopShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	topCommand.Flags().SetInterspersed(false)
	return topCommand
}

func topAction(cmd *cobra.Command, args []string) error {
	// NOTE: rootless container does not rely on cgroupv1.
	// more details about possible ways to resolve this concern: #223
	if rootlessutil.IsRootless() && infoutil.CgroupsVersion() == "1" {
		return fmt.Errorf("top requires cgroup v2 for rootless containers, see https://rootlesscontaine.rs/getting-started/common/cgroup2/")
	}

	cgroupManager, err := cmd.Flags().GetString("cgroup-manager")
	if err != nil {
		return err
	}
	if cgroupManager == "none" {
		return errors.New("cgroup manager must not be \"none\"")
	}

	client, ctx, cancel, err := ncclient.NewClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			if err := utils.ContainerTop(ctx, cmd, client, found.Container.ID(), strings.Join(args[1:], " ")); err != nil {
				return err
			}
			return nil
		},
	}

	n, err := walker.Walk(ctx, args[0])
	if err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("no such container %s", args[0])
	}
	return nil
}
