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
	"fmt"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/cmd/compose"
	"github.com/containerd/nerdctl/pkg/cmd/container"
	"github.com/containerd/nerdctl/pkg/containerutil"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/spf13/cobra"
)

func newComposeTopCommand() *cobra.Command {
	var composeTopCommand = &cobra.Command{
		Use:                   "top [SERVICE...]",
		Short:                 "Display the running processes of service containers",
		RunE:                  composeTopAction,
		SilenceUsage:          true,
		SilenceErrors:         true,
		DisableFlagsInUseLine: true,
	}
	return composeTopCommand
}

func composeTopAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()
	options, err := getComposeOptions(cmd, globalOptions.DebugFull, globalOptions.Experimental)
	if err != nil {
		return err
	}
	c, err := compose.New(client, globalOptions, options, cmd.OutOrStdout(), cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	serviceNames, err := c.ServiceNames(args...)
	if err != nil {
		return err
	}
	containers, err := c.Containers(ctx, serviceNames...)
	if err != nil {
		return err
	}
	stdout := cmd.OutOrStdout()
	for _, c := range containers {
		cStatus, err := containerutil.ContainerStatus(ctx, c)
		if err != nil {
			return err
		}
		if cStatus.Status != containerd.Running {
			continue
		}

		info, err := c.Info(ctx, containerd.WithoutRefreshedMetadata)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, info.Labels[labels.Name])
		// `compose ps` uses empty ps args
		err = container.Top(ctx, client, []string{c.ID()}, types.ContainerTopOptions{
			Stdout:   cmd.OutOrStdout(),
			GOptions: globalOptions,
		})
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout)
	}

	return nil
}
