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
	"github.com/urfave/cli/v2"
)

var imageCommand = &cli.Command{
	Name:     "image",
	Usage:    "Manage images",
	Category: CategoryManagement,
	Subcommands: []*cli.Command{
		buildCommand,
		// commitCommand is in "container", not in "image"
		imageLsCommand(),
		pullCommand,
		pushCommand,
		loadCommand,
		saveCommand,
		tagCommand,
		imageRmCommand(),
		imageConvertCommand,
		imageInspectCommand,
	},
}

func imageLsCommand() *cli.Command {
	x := *imagesCommand
	x.Name = "ls"
	return &x
}

func imageRmCommand() *cli.Command {
	x := *rmiCommand
	x.Name = "rm"
	return &x
}
