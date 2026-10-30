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
	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/compose"
	"github.com/containerd/nerdctl/v2/pkg/composer"
	"github.com/containerd/nerdctl/v2/pkg/errutil"
)

func waitCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:           "wait [flags] SERVICE [SERVICE...]",
		Short:         "Block until the containers of the given services stop.",
		Args:          cobra.MinimumNArgs(1),
		RunE:          waitAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.Flags().Bool("down-project", false, "Drop the project when the containers stop")
	return cmd
}

func waitAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	downProject, err := cmd.Flags().GetBool("down-project")
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
	code, err := c.Wait(ctx, args, composer.WaitOptions{DownProject: downProject}, cmd.OutOrStdout())
	if err != nil {
		return err
	}
	if code != 0 {
		return errutil.NewExitCoderErr(int(code))
	}
	return nil
}
