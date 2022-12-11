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

package container

import (
	"github.com/containerd/nerdctl/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/cmd/nerdctl/container/cp"
	"github.com/containerd/nerdctl/cmd/nerdctl/container/exec"
	"github.com/containerd/nerdctl/cmd/nerdctl/ps"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/common"
	"github.com/spf13/cobra"
)

func NewContainerCommand() *cobra.Command {
	containerCommand := &cobra.Command{
		Annotations:   map[string]string{common.Category: common.Management},
		Use:           "container",
		Short:         "Manage Containers",
		RunE:          completion.UnknownSubcommandAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	containerCommand.AddCommand(
		NewCreateCommand(),
		NewRunCommand(),
		NewUpdateCommand(),
		exec.NewExecCommand(),
		LsCommand(),
		NewContainerInspectCommand(),
		NewLogsCommand(),
		NewPortCommand(),
		NewRmCommand(),
		NewStopCommand(),
		NewStartCommand(),
		NewRestartCommand(),
		NewKillCommand(),
		NewPauseCommand(),
		NewWaitCommand(),
		NewUnpauseCommand(),
		NewCommitCommand(),
		NewRenameCommand(),
		NewContainerPruneCommand(),
	)
	cp.AddCpCommand(containerCommand)
	return containerCommand
}

func LsCommand() *cobra.Command {
	x := ps.NewPsCommand()
	x.Use = "ls"
	x.Aliases = []string{"list"}
	return x
}
