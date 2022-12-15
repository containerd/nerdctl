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

package network

import (
	"github.com/containerd/nerdctl/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/common"
	"github.com/spf13/cobra"
)

func NewNetworkCommand() *cobra.Command {
	networkCommand := &cobra.Command{
		Annotations:   map[string]string{common.Category: common.Management},
		Use:           "network",
		Short:         "Manage networks",
		RunE:          completion.UnknownSubcommandAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	networkCommand.AddCommand(
		NewLsCommand(),
		NewInspectCommand(),
		NewCreateCommand(),
		NewRmCommand(),
		NewPruneCommand(),
	)
	return networkCommand
}
