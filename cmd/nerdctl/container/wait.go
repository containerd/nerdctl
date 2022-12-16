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
	"io"

	"github.com/containerd/containerd"
	ncclient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"
)

func NewWaitCommand() *cobra.Command {
	var waitCommand = &cobra.Command{
		Use:               "wait [flags] CONTAINER [CONTAINER, ...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Block until one or more containers stop, then print their exit codes.",
		RunE:              containerWaitAction,
		ValidArgsFunction: waitShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	return waitCommand
}

func containerWaitAction(cmd *cobra.Command, args []string) error {
	client, ctx, cancel, err := ncclient.New(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	var containers []containerd.Container
	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			containers = append(containers, found.Container)
			return nil
		},
	}

	for _, req := range args {
		n, err := walker.Walk(ctx, req)
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("no such container: %s", req)
		}
	}
	var allErr error
	w := cmd.OutOrStdout()
	for _, container := range containers {
		if waitErr := waitContainer(ctx, w, container); waitErr != nil {
			allErr = multierror.Append(allErr, waitErr)
		}
	}
	return allErr
}

func waitContainer(ctx context.Context, w io.Writer, container containerd.Container) error {
	task, err := container.Task(ctx, nil)
	if err != nil {
		return err
	}

	statusC, err := task.Wait(ctx)
	if err != nil {
		return err
	}

	status := <-statusC
	code, _, err := status.Result()
	if err != nil {
		return err
	}
	fmt.Fprintln(w, code)
	return nil
}

func waitShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show running container names
	statusFilterFn := func(st containerd.ProcessStatus) bool {
		return st == containerd.Running
	}
	return completion.ShellCompleteContainerNames(cmd, statusFilterFn)
}
