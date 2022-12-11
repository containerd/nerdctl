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

package image

import (
	"github.com/containerd/nerdctl/cmd/nerdctl/build"
	"github.com/containerd/nerdctl/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/cmd/nerdctl/history"
	"github.com/containerd/nerdctl/cmd/nerdctl/load"
	"github.com/containerd/nerdctl/cmd/nerdctl/pull"
	"github.com/containerd/nerdctl/cmd/nerdctl/push"
	"github.com/containerd/nerdctl/cmd/nerdctl/rmi"
	"github.com/containerd/nerdctl/cmd/nerdctl/save"
	"github.com/containerd/nerdctl/cmd/nerdctl/tag"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/common"
	"github.com/spf13/cobra"
)

func NewImageCommand() *cobra.Command {
	cmd := &cobra.Command{
		Annotations:   map[string]string{common.Category: common.Management},
		Use:           "image",
		Short:         "Manage images",
		RunE:          completion.UnknownSubcommandAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(
		build.NewBuildCommand(),
		// commitCommand is in "container", not in "image"
		LsCommand(),
		history.NewHistoryCommand(),
		pull.NewPullCommand(),
		push.NewPushCommand(),
		load.NewLoadCommand(),
		save.NewSaveCommand(),
		tag.NewTagCommand(),
		RmCommand(),
		NewImageConvertCommand(),
		NewImageInspectCommand(),
		NewImageEncryptCommand(),
		NewImageDecryptCommand(),
		NewImagePruneCommand(),
	)
	return cmd
}

func LsCommand() *cobra.Command {
	x := NewImagesCommand()
	x.Use = "ls"
	x.Aliases = []string{"list"}
	return x
}

func RmCommand() *cobra.Command {
	x := rmi.NewRmiCommand()
	x.Use = "rm"
	x.Aliases = []string{"remove"}
	return x
}
