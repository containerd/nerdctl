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
	"github.com/containerd/nerdctl/pkg/composer"
	"github.com/spf13/cobra"
)

func newComposePullCommand() *cobra.Command {
	var composePullCommand = &cobra.Command{
		Use:           "pull [SERVICE...]",
		Short:         "Pull service images",
		RunE:          composePullAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	composePullCommand.Flags().BoolP("quiet", "q", false, "Pull without printing progress information")
	return composePullCommand
}

func composePullAction(cmd *cobra.Command, args []string) error {
	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	c, err := getComposer(cmd, client)
	if err != nil {
		return err
	}
	quiet, err := cmd.Flags().GetBool("quiet")
	if err != nil {
		return err
	}
	po := composer.PullOptions{
		Quiet: quiet,
	}
	return c.Pull(ctx, po, args)
}
