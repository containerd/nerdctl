/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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
	"github.com/containerd/nerdctl/pkg/mountutil/volumestore"
	"github.com/urfave/cli/v2"
)

var volumeCommand = &cli.Command{
	Name:     "volume",
	Usage:    "Manage volumes",
	Category: CategoryManagement,
	Subcommands: []*cli.Command{
		volumeLsCommand,
		volumeInspectCommand,
		volumeCreateCommand,
		volumeRmCommand,
	},
}

// getVolumeStore returns a string like `/var/lib/nerdctl/1935db59/volumes/default`
func getVolumeStore(clicontext *cli.Context) (string, error) {
	dataStore, err := getDataStore(clicontext)
	if err != nil {
		return "", err
	}
	ns := clicontext.String("namespace")
	return volumestore.New(dataStore, ns)
}
