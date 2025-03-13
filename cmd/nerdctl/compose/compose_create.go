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
	"errors"

	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/compose"
	"github.com/containerd/nerdctl/v2/pkg/composer"
)

func createCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:           "create [flags] [SERVICE...]",
		Short:         "Creates containers for one or more services",
		RunE:          createAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.Flags().Bool("build", false, "Build images before starting containers.")
	cmd.Flags().Bool("no-build", false, "Don't build an image even if it's missing, conflict with --build.")
	cmd.Flags().Bool("force-recreate", false, "Recreate containers even if their configuration and image haven't changed.")
	cmd.Flags().Bool("no-recreate", false, "Don't recreate containers if they exist, conflict with --force-recreate.")
	cmd.Flags().String("pull", "missing", "Pull images before running. (support always|missing|never)")
	return cmd
}

func createAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	build, err := cmd.Flags().GetBool("build")
	if err != nil {
		return err
	}
	noBuild, err := cmd.Flags().GetBool("no-build")
	if err != nil {
		return err
	}
	if build && noBuild {
		return errors.New("flag --build and --no-build cannot be specified together")
	}
	forceRecreate, err := cmd.Flags().GetBool("force-recreate")
	if err != nil {
		return err
	}
	noRecreate, err := cmd.Flags().GetBool("no-recreate")
	if err != nil {
		return err
	}
	if forceRecreate && noRecreate {
		return errors.New("flag --force-recreate and --no-recreate cannot be specified together")
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

	opt := composer.CreateOptions{
		Build:         build,
		NoBuild:       noBuild,
		ForceRecreate: forceRecreate,
		NoRecreate:    noRecreate,
	}

	if cmd.Flags().Changed("pull") {
		pull, err := cmd.Flags().GetString("pull")
		if err != nil {
			return err
		}
		opt.Pull = &pull
	}

	return c.Create(ctx, opt, args)
}
