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
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/cmd/image"

	"github.com/spf13/cobra"
)

func newTagCommand() *cobra.Command {
	var tagCommand = &cobra.Command{
		Use:               "tag [flags] SOURCE_IMAGE[:TAG] TARGET_IMAGE[:TAG]",
		Short:             "Create a tag TARGET_IMAGE that refers to SOURCE_IMAGE",
		Args:              IsExactArgs(2),
		RunE:              tagAction,
		ValidArgsFunction: tagShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	return tagCommand
}

func tagAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	return image.Tag(cmd.Context(), types.ImageTagCommandOptions{
		GOptions: globalOptions,
		Source:   args[0],
		Target:   args[1],
	})
}

func tagShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) < 2 {
		// show image names
		return shellCompleteImageNames(cmd)
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}
