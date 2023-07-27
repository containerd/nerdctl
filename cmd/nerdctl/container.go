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
	"github.com/spf13/cobra"
)

func newContainerCommand() *cobra.Command {
	containerCommand := &cobra.Command{
		Annotations:   map[string]string{Category: Management},
		Use:           "container",
		Short:         "Manage containers",
		RunE:          unknownSubcommandAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	containerCommand.AddCommand(
		newCreateCommand(),
		newRunCommand(),
		newUpdateCommand(),
		newExecCommand(),
		containerLsCommand(),
		newContainerInspectCommand(),
		newLogsCommand(),
		newPortCommand(),
		newRmCommand(),
		newStopCommand(),
		newStartCommand(),
		newRestartCommand(),
		newKillCommand(),
		newPauseCommand(),
		newWaitCommand(),
		newUnpauseCommand(),
		newCommitCommand(),
		newRenameCommand(),
		newContainerPruneCommand(),
		newStatsCommand(),
	)
	addCpCommand(containerCommand)
	return containerCommand
}

func containerLsCommand() *cobra.Command {
	x := newPsCommand()
	x.Use = "ls"
	x.Aliases = []string{"list"}
	return x
}
