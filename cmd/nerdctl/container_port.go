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
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/containerutil"
	"github.com/containerd/nerdctl/v2/pkg/idutil/containerwalker"
)

func newPortCommand() *cobra.Command {
	var portCommand = &cobra.Command{
		Use:               "port [flags] CONTAINER [PRIVATE_PORT[/PROTO]]",
		Args:              cobra.RangeArgs(1, 2),
		Short:             "List port mappings or a specific mapping for the container",
		RunE:              portAction,
		ValidArgsFunction: portShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	return portCommand
}

func portAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	argPort := -1
	argProto := ""
	portProto := ""
	if len(args) == 2 {
		portProto = args[1]
	}

	if portProto != "" {
		splitBySlash := strings.Split(portProto, "/")
		var err error
		argPort, err = strconv.Atoi(splitBySlash[0])
		if err != nil {
			return err
		}
		if argPort <= 0 {
			return fmt.Errorf("unexpected port %d", argPort)
		}
		switch len(splitBySlash) {
		case 1:
			argProto = "tcp"
		case 2:
			argProto = strings.ToLower(splitBySlash[1])
		default:
			return fmt.Errorf("failed to parse %q", portProto)
		}
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
			return containerutil.PrintHostPort(ctx, cmd.OutOrStdout(), found.Container, argPort, argProto)
		},
	}
	req := args[0]
	n, err := walker.Walk(ctx, req)
	if err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("no such container %s", req)
	}
	return nil
}

func portShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return shellCompleteContainerNames(cmd, nil)
}
