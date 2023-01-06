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
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/cmd/compose"
	"github.com/containerd/nerdctl/pkg/composer"
	"github.com/spf13/cobra"
)

func newComposeKillCommand() *cobra.Command {
	var composeKillCommand = &cobra.Command{
		Use:           "kill [flags] [SERVICE...]",
		Short:         "Force stop service containers",
		RunE:          composeKillAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	composeKillCommand.Flags().StringP("signal", "s", "SIGKILL", "SIGNAL to send to the container.")
	return composeKillCommand
}

func composeKillAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	signal, err := cmd.Flags().GetString("signal")
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

	killOpts := composer.KillOptions{
		Signal: signal,
	}
	return c.Kill(ctx, killOpts, args)
}
