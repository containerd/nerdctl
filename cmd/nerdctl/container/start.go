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
	"github.com/spf13/cobra"
)

func NewStartCommand() *cobra.Command {
	var startCommand = &cobra.Command{
		Use:               "start [flags] CONTAINER [CONTAINER, ...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "start one or more running containers",
		RunE:              startAction,
		ValidArgsFunction: completion.StartShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}

	startCommand.Flags().SetInterspersed(false)
	startCommand.Flags().BoolP("attach", "a", false, "Attach STDOUT/STDERR and forward signals")

	return startCommand
}

func startAction(cmd *cobra.Command, args []string) error {
	client, ctx, cancel, err := ncclient.NewClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	flagA, err := cmd.Flags().GetBool("attach")
	if err != nil {
		return err
	}

	if flagA && len(args) > 1 {
		return fmt.Errorf("you cannot start and attach multiple containers at once")
	}

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			if err := action.StartContainer(ctx, found.Container, flagA, client); err != nil {
				return err
			}
			if !flagA {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\n", found.Req)
				if err != nil {
					return err
				}
			}
			return err
		},
	}
	for _, req := range args {
		n, err := walker.Walk(ctx, req)
		if err != nil {
			return err
		} else if n == 0 {
			return fmt.Errorf("no such container %s", req)
		}
	}
	return nil
}
