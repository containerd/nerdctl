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

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
)

func newSystemCommand() *cobra.Command {
	var systemCommand = &cobra.Command{
		Annotations:   map[string]string{helpers.Category: helpers.Management},
		Use:           "system",
		Short:         "Manage containerd",
		RunE:          unknownSubcommandAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	// versionCommand is not here
	systemCommand.AddCommand(
		newEventsCommand(),
		newInfoCommand(),
		newSystemPruneCommand(),
	)
	return systemCommand
}
