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

func newComposeDownCommand() *cobra.Command {
	var composeDownCommand = &cobra.Command{
		Use:           "down",
		Short:         "Remove containers and associated resources",
		Args:          cobra.NoArgs,
		RunE:          composeDownAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	composeDownCommand.Flags().BoolP("volumes", "v", false, "Remove named volumes declared in the `volumes` section of the Compose file and anonymous volumes attached to containers.")
	composeDownCommand.Flags().Bool("remove-orphans", false, "Remove containers for services not defined in the Compose file.")
	return composeDownCommand
}

func composeDownAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	volumes, err := cmd.Flags().GetBool("volumes")
	if err != nil {
		return err
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return err
	}

	removeOrphans, err := cmd.Flags().GetBool("remove-orphans")
	if err != nil {
		return err
	}
	defer cancel()

	c, err := getComposer(cmd, client, globalOptions)
	if err != nil {
		return err
	}
	downOpts := composer.DownOptions{
		RemoveVolumes: volumes,
		RemoveOrphans: removeOrphans,
	}
	return c.Down(ctx, downOpts)
}
