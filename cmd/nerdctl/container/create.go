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
	"fmt"
	"runtime"

	ncclient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/container"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/run"
	"github.com/spf13/cobra"
)

func NewCreateCommand() *cobra.Command {
	shortHelp := "Create a new container. Optionally specify \"ipfs://\" or \"ipns://\" scheme to pull image from IPFS."
	longHelp := shortHelp
	switch runtime.GOOS {
	case "windows":
		longHelp += "\n"
		longHelp += "WARNING: `nerdctl create` is experimental on Windows and currently broken (https://github.com/containerd/nerdctl/issues/28)"
	case "freebsd":
		longHelp += "\n"
		longHelp += "WARNING: `nerdctl create` is experimental on FreeBSD and currently requires `--net=none` (https://github.com/containerd/nerdctl/blob/main/docs/freebsd.md)"
	}
	var createCommand = &cobra.Command{
		Use:               "create [flags] IMAGE [COMMAND] [ARG...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             shortHelp,
		Long:              longHelp,
		RunE:              createAction,
		ValidArgsFunction: completion.RunShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	createCommand.Flags().SetInterspersed(false)
	run.SetCreateFlags(createCommand)
	return createCommand
}

func createAction(cmd *cobra.Command, args []string) error {
	platform, err := cmd.Flags().GetString("platform")
	if err != nil {
		return err
	}

	experimental, err := cmd.Flags().GetBool("experimental")
	if err != nil {
		return err
	}

	if (platform == "windows" || platform == "freebsd") && !experimental {
		return fmt.Errorf("%s requires experimental mode to be enabled", platform)
	}

	client, ctx, cancel, err := ncclient.NewWithPlatform(cmd, platform)
	if err != nil {
		return err
	}
	defer cancel()

	container, gc, err := container.CreateContainer(ctx, cmd, client, args, platform, false, false, true)
	if err != nil {
		if gc != nil {
			gc()
		}
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), container.ID())
	return nil
}
