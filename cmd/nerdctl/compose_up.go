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

func newComposeUpCommand() *cobra.Command {
	var composeUpCommand = &cobra.Command{
		Use:           "up",
		Short:         "Create and start containers",
		RunE:          composeUpAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	composeUpCommand.Flags().BoolP("detach", "d", false, "Detached mode: Run containers in the background")
	composeUpCommand.Flags().Bool("no-color", false, "Produce monochrome output")
	composeUpCommand.Flags().Bool("no-log-prefix", false, "Don't print prefix in logs")
	composeUpCommand.Flags().Bool("build", false, "Build images before starting containers.")
	return composeUpCommand
}

func composeUpAction(cmd *cobra.Command, args []string) error {
	detach, err := cmd.Flags().GetBool("detach")
	if err != nil {
		return err
	}
	noColor, err := cmd.Flags().GetBool("no-color")
	if err != nil {
		return err
	}
	noLogPrefix, err := cmd.Flags().GetBool("no-log-prefix")
	if err != nil {
		return err
	}
	build, err := cmd.Flags().GetBool("build")
	if err != nil {
		return err
	}

	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	c, err := getComposer(cmd, client)
	if err != nil {
		return err
	}
	uo := composer.UpOptions{
		Detach:      detach,
		NoColor:     noColor,
		NoLogPrefix: noLogPrefix,
		ForceBuild:  build,
	}
	return c.Up(ctx, uo)
}
