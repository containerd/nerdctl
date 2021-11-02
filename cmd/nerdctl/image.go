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

func newImageCommand() *cobra.Command {
	cmd := &cobra.Command{
		Category:      CategoryManagement,
		Use:           "image",
		Short:         "Manage images",
		RunE:          unknownSubcommandAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(
		newBuildCommand(),
		// commitCommand is in "container", not in "image"
		imageLsCommand(),
		newPullCommand(),
		newPushCommand(),
		newLoadCommand(),
		newSaveCommand(),
		newTagCommand(),
		imageRmCommand(),
		newImageConvertCommand(),
		newImageInspectCommand(),
		newImageEncryptCommand(),
		newImageDecryptCommand(),
	)
	return cmd
}

func imageLsCommand() *cobra.Command {
	x := newImagesCommand()
	x.Use = "ls"
	return x
}

func imageRmCommand() *cobra.Command {
	x := newRmiCommand()
	x.Use = "rm"
	return x
}
