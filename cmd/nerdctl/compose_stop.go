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
	"github.com/containerd/nerdctl/pkg/composer"
	"github.com/spf13/cobra"
)

func newComposeStopCommand() *cobra.Command {
	var composeStopCommand = &cobra.Command{
		Use:           "stop [flags] [SERVICE...]",
		Short:         "Stop running containers without removing them.",
		RunE:          composeStopAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	composeStopCommand.Flags().UintP("timeout", "t", 10, "Seconds to wait for stop before killing them")
	return composeStopCommand
}

func composeStopAction(cmd *cobra.Command, args []string) error {
	var opt composer.StopOptions

	if cmd.Flags().Changed("timeout") {
		timeValue, err := cmd.Flags().GetUint("timeout")
		if err != nil {
			return err
		}
		opt.Timeout = &timeValue
	}
	namespace, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	}
	address, err := cmd.Flags().GetString("address")
	if err != nil {
		return err
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), namespace, address)
	if err != nil {
		return err
	}
	defer cancel()

	c, err := getComposer(cmd, client)
	if err != nil {
		return err
	}
	return c.Stop(ctx, opt, args)
}
