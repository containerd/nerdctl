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

	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/compose"
	"github.com/containerd/nerdctl/v2/pkg/composer"
)

func configCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:           "config",
		Short:         "Validate and view the Compose file",
		RunE:          configAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.Flags().BoolP("quiet", "q", false, "Only validate the configuration, don't print anything.")
	cmd.Flags().Bool("services", false, "Print the service names, one per line.")
	cmd.Flags().Bool("volumes", false, "Print the volume names, one per line.")
	cmd.Flags().String("hash", "", "Print the service config hash, one per line.")
	cmd.RegisterFlagCompletionFunc("hash", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"\"*\""}, cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}

func configAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return err
	}
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
