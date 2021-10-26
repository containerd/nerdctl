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
	"github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/mountutil/volumestore"
	"github.com/spf13/cobra"
)

func newVolumeCommand() *cobra.Command {
	volumeCommand := &cobra.Command{
		Annotations:   map[string]string{Category: Management},
		Use:           "volume",
		Short:         "Manage volumes",
		RunE:          unknownSubcommandAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	volumeCommand.AddCommand(
		newVolumeLsCommand(),
		newVolumeInspectCommand(),
		newVolumeCreateCommand(),
		newVolumeRmCommand(),
	)
	return volumeCommand
}

// getVolumeStore returns a volume store
// that corresponds to a directory like `/var/lib/nerdctl/1935db59/volumes/default`
func getVolumeStore(cmd *cobra.Command) (volumestore.VolumeStore, error) {
	ns, err := defaults.GetglobalString(cmd, "namespace")
	if err != nil {
		return nil, err
	}
	dataStore, err := getDataStore(cmd)
	if err != nil {
		return nil, err
	}
	return volumestore.New(dataStore, ns)
}
