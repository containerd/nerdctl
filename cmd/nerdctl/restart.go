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
	"fmt"
	"time"

	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/spf13/cobra"
)

func newRestartCommand() *cobra.Command {
	var restartCommand = &cobra.Command{
		Use:               "restart [flags] CONTAINER [CONTAINER, ...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Restart one or more running containers",
		RunE:              restartAction,
		ValidArgsFunction: startShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	restartCommand.Flags().UintP("time", "t", 10, "Seconds to wait for stop before killing it")
	return restartCommand
}

func restartAction(cmd *cobra.Command, args []string) error {
	// Time to wait after sending a SIGTERM and before sending a SIGKILL.
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	var timeout *time.Duration
	if cmd.Flags().Changed("time") {
		timeValue, err := cmd.Flags().GetUint("time")
		if err != nil {
			return err
		}
		t := time.Duration(timeValue) * time.Second
		timeout = &t
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
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
			if err := stopContainer(ctx, found.Container, timeout); err != nil {
				return err
			}
			if err := startContainer(ctx, found.Container, false, client); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", found.Req)
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
