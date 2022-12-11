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

package compose

import (
	nerdClient "github.com/containerd/nerdctl/cmd/nerdctl/client"
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
	signal, err := cmd.Flags().GetString("signal")
	if err != nil {
		return err
	}
	client, ctx, cancel, err := nerdClient.NewClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()
	c, err := getComposer(cmd, client)
	if err != nil {
		return err
	}
	killOpts := composer.KillOptions{
		Signal: signal,
	}
	return c.Kill(ctx, killOpts, args)
}
