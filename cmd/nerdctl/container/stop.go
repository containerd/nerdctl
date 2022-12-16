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
	"time"

	ncclient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/action"
	"github.com/spf13/cobra"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
)

func NewStopCommand() *cobra.Command {
	var stopCommand = &cobra.Command{
		Use:               "stop [flags] CONTAINER [CONTAINER, ...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Stop one or more running containers",
		RunE:              stopAction,
		ValidArgsFunction: completion.StopShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	stopCommand.Flags().IntP("time", "t", 10, "Seconds to wait for stop before killing it")
	return stopCommand
}

func stopAction(cmd *cobra.Command, args []string) error {
	// Time to wait after sending a SIGTERM and before sending a SIGKILL.
	var timeout *time.Duration
	if cmd.Flags().Changed("time") {
		timeValue, err := cmd.Flags().GetInt("time")
		if err != nil {
			return err
		}
		t := time.Duration(timeValue) * time.Second
		timeout = &t
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
			if err := action.StopContainer(ctx, found.Container, timeout); err != nil {
				if errdefs.IsNotFound(err) {
					fmt.Fprintf(cmd.ErrOrStderr(), "No such container: %s\n", found.Req)
					return nil
				}
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
