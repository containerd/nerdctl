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
	"context"
	"fmt"

	ncclient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/action"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func NewRmCommand() *cobra.Command {
	var rmCommand = &cobra.Command{
		Use:               "rm [flags] CONTAINER [CONTAINER, ...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Remove one or more containers",
		RunE:              rmAction,
		ValidArgsFunction: rmShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	rmCommand.Flags().BoolP("force", "f", false, "Force the removal of a running|paused|unknown container (uses SIGKILL)")
	rmCommand.Flags().BoolP("volumes", "v", false, "Remove volume associated with the container")
	return rmCommand
}

func rmAction(cmd *cobra.Command, args []string) error {
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}
	removeAnonVolumes, err := cmd.Flags().GetBool("volumes")
	if err != nil {
		return err
	}

	client, ctx, cancel, err := ncclient.New(cmd)
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
			if err := action.RemoveContainer(ctx, cmd, found.Container, force, removeAnonVolumes); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", found.Req)
			return err
		},
	}
	for _, req := range args {
		n, err := walker.Walk(ctx, req)
		if err == nil && n == 0 {
			err = fmt.Errorf("no such container %s", req)
		}
		if err != nil {
			if force {
				logrus.Error(err)
			} else {
				return err
			}
		}
	}
	return nil
}

func rmShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show container names
	return completion.ShellCompleteContainerNames(cmd, nil)
}
