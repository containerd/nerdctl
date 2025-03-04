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
)

func buildCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:           "build [flags] [SERVICE...]",
		Short:         "Build or rebuild services",
		RunE:          buildAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.Flags().StringArray("build-arg", nil, "Set build-time variables for services.")
	cmd.Flags().Bool("no-cache", false, "Do not use cache when building the image.")
	cmd.Flags().String("progress", "", "Set type of progress output (auto, plain, tty). Use plain to show container output")

	return cmd
}

func buildAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	buildArg, err := cmd.Flags().GetStringArray("build-arg")
	if err != nil {
		return err
	}
	noCache, err := cmd.Flags().GetBool("no-cache")
	if err != nil {
		return err
	}
	progress, err := cmd.Flags().GetString("progress")
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
	bo := composer.BuildOptions{
		Args:     buildArg,
		NoCache:  noCache,
		Progress: progress,
	}
	return c.Build(ctx, bo, args)
}
