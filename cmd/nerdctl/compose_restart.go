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
	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/compose"
	"github.com/containerd/nerdctl/v2/pkg/composer"
)

func newComposeRestartCommand() *cobra.Command {
	var composeRestartCommand = &cobra.Command{
		Use:           "restart [flags] [SERVICE...]",
		Short:         "Restart containers of given (or all) services",
		RunE:          composeRestartAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	composeRestartCommand.Flags().UintP("timeout", "t", 10, "Seconds to wait before restarting them")
	return composeRestartCommand
}

func composeRestartAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	var opt composer.RestartOptions

	if cmd.Flags().Changed("timeout") {
		timeValue, err := cmd.Flags().GetUint("timeout")
		if err != nil {
			return err
		}
		opt.Timeout = &timeValue
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
	return c.Restart(ctx, opt, args)
}
