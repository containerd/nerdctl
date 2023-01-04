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
	"errors"

	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/composer"
	"github.com/spf13/cobra"
)

func newComposeCreateCommand() *cobra.Command {
	var composeCreateCommand = &cobra.Command{
		Use:           "create [flags] [SERVICE...]",
		Short:         "Creates containers for one or more services",
		RunE:          composeCreateAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	composeCreateCommand.Flags().Bool("build", false, "Build images before starting containers.")
	composeCreateCommand.Flags().Bool("no-build", false, "Don't build an image even if it's missing, conflict with --build.")
	composeCreateCommand.Flags().Bool("force-recreate", false, "Recreate containers even if their configuration and image haven't changed.")
	composeCreateCommand.Flags().Bool("no-recreate", false, "Don't recreate containers if they exist, conflict with --force-recreate.")
	composeCreateCommand.Flags().String("pull", "missing", "Pull images before running. (support always|missing|never)")
	return composeCreateCommand
}

func composeCreateAction(cmd *cobra.Command, args []string) error {
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
		return nil
	}
	if forceRecreate && noRecreate {
		return errors.New("flag --force-recreate and --no-recreate cannot be specified together")
	}

	globalOptions, err := processGlobalFlag(cmd)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	c, err := getComposer(cmd, client, globalOptions)
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
