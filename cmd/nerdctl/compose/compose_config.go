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
	"fmt"

	nerdClient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/pkg/composer"
	"github.com/spf13/cobra"
)

func newComposeConfigCommand() *cobra.Command {
	var composeConfigCommand = &cobra.Command{
		Use:           "config",
		Short:         "Validate and view the Compose file",
		RunE:          composeConfigAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	composeConfigCommand.Flags().BoolP("quiet", "q", false, "Only validate the configuration, don't print anything.")
	composeConfigCommand.Flags().Bool("services", false, "Print the service names, one per line.")
	composeConfigCommand.Flags().Bool("volumes", false, "Print the volume names, one per line.")
	composeConfigCommand.Flags().String("hash", "", "Print the service config hash, one per line.")
	composeConfigCommand.RegisterFlagCompletionFunc("hash", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"\"*\""}, cobra.ShellCompDirectiveNoFileComp
	})
	return composeConfigCommand
}

func composeConfigAction(cmd *cobra.Command, args []string) error {
	if len(args) != 0 {
		// TODO: support specifying service names as args
		return fmt.Errorf("arguments %v not supported", args)
	}
	quiet, err := cmd.Flags().GetBool("quiet")
	if err != nil {
		return err
	}
	services, err := cmd.Flags().GetBool("services")
	if err != nil {
		return err
	}
	volumes, err := cmd.Flags().GetBool("volumes")
	if err != nil {
		return err
	}
	hash, err := cmd.Flags().GetString("hash")
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
	if quiet {
		return nil
	}
	co := composer.ConfigOptions{
		Services: services,
		Volumes:  volumes,
		Hash:     hash,
	}
	return c.Config(ctx, cmd.OutOrStdout(), co)
}
