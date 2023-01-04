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
	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/cmd/compose"
	"github.com/containerd/nerdctl/pkg/composer"

	"github.com/spf13/cobra"
)

func newComposeCommand() *cobra.Command {
	var composeCommand = &cobra.Command{
		Use:              "compose [flags] COMMAND",
		Short:            "Compose",
		RunE:             unknownSubcommandAction,
		SilenceUsage:     true,
		SilenceErrors:    true,
		TraverseChildren: true, // required for global short hands like -f
	}
	// `-f` is a nonPersistentAlias, as it conflicts with `nerdctl compose logs --follow`
	AddPersistentStringArrayFlag(composeCommand, "file", nil, []string{"f"}, nil, "", "Specify an alternate compose file")
	composeCommand.PersistentFlags().String("project-directory", "", "Specify an alternate working directory")
	composeCommand.PersistentFlags().StringP("project-name", "p", "", "Specify an alternate project name")
	composeCommand.PersistentFlags().String("env-file", "", "Specify an alternate environment file")

	composeCommand.AddCommand(
		newComposeUpCommand(),
		newComposeLogsCommand(),
		newComposeConfigCommand(),
		newComposeBuildCommand(),
		newComposeExecCommand(),
		newComposeImagesCommand(),
		newComposePortCommand(),
		newComposePushCommand(),
		newComposePullCommand(),
		newComposeDownCommand(),
		newComposePsCommand(),
		newComposeKillCommand(),
		newComposeRestartCommand(),
		newComposeRemoveCommand(),
		newComposeRunCommand(),
		newComposeVersionCommand(),
		newComposeStartCommand(),
		newComposeStopCommand(),
		newComposePauseCommand(),
		newComposeUnpauseCommand(),
		newComposeTopCommand(),
		newComposeCreateCommand(),
	)

	return composeCommand
}

func getComposer(cmd *cobra.Command, client *containerd.Client, globalOptions *types.GlobalCommandOptions) (*composer.Composer, error) {
	options, err := getComposeOptions(cmd)
	if err != nil {
		return nil, err
	}
	volStore, err := getVolumeStore(cmd)
	if err != nil {
		return nil, err
	}
	return compose.GetComposer(options, client, volStore, cmd.OutOrStdout(), cmd.ErrOrStderr(), globalOptions)
}

func getComposeOptions(cmd *cobra.Command) (*composer.Options, error) {
	options := &composer.Options{}
	options.NerdctlCmd, options.NerdctlArgs = globalFlags(cmd)
	var err error
	options.ProjectDirectory, err = cmd.Flags().GetString("project-directory")
	if err != nil {
		return nil, err
	}
	options.EnvFile, err = cmd.Flags().GetString("env-file")
	if err != nil {
		return nil, err
	}
	options.Project, err = cmd.Flags().GetString("project-name")
	if err != nil {
		return nil, err
	}
	options.DebugPrintFull, err = cmd.Flags().GetBool("debug-full")
	if err != nil {
		return nil, err
	}
	options.ConfigPaths, err = cmd.Flags().GetStringArray("file")
	if err != nil {
		return nil, err
	}
	options.Experimental, err = cmd.Flags().GetBool("experimental")
	if err != nil {
		return nil, err
	}
	return options, nil
}
