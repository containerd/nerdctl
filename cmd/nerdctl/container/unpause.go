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

func NewUnpauseCommand() *cobra.Command {
	var unpauseCommand = &cobra.Command{
		Use:               "unpause [flags] CONTAINER [CONTAINER, ...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Unpause all processes within one or more containers",
		RunE:              unpauseAction,
		ValidArgsFunction: completion.UnpauseShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	return unpauseCommand
}

func unpauseAction(cmd *cobra.Command, args []string) error {
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
			if err := action.UnpauseContainer(ctx, client, found.Container.ID()); err != nil {
				return err
			}

			_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\n", found.Req)
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
