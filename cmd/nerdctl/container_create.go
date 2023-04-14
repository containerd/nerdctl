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
	"fmt"
	"runtime"

	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/containerutil"
	"github.com/spf13/cobra"
)

func newCreateCommand() *cobra.Command {
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
		ValidArgsFunction: runShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	createCommand.Flags().SetInterspersed(false)
	setCreateFlags(createCommand)
	return createCommand
}

func createAction(cmd *cobra.Command, args []string) (err error) {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	platform, err := cmd.Flags().GetString("platform")
	if err != nil {
		return err
	}
	if (platform == "windows" || platform == "freebsd") && !globalOptions.Experimental {
		return fmt.Errorf("%s requires experimental mode to be enabled", platform)
	}
	client, ctx, cancel, err := clientutil.NewClientWithPlatform(cmd.Context(), globalOptions.Namespace, globalOptions.Address, platform)
	if err != nil {
		return err
	}
	defer cancel()

	flagT, err := cmd.Flags().GetBool("tty")
	if err != nil {
		return err
	}

	netFlags, err := loadNetworkFlags(cmd)
	if err != nil {
		return fmt.Errorf("failed to load networking flags: %s", err)
	}

	netManager, err := containerutil.NewNetworkingOptionsManager(globalOptions, netFlags)
	if err != nil {
		return err
	}

	container, gc, err := createContainer(ctx, cmd, client, netManager, globalOptions, args, platform, false, flagT, true)
	if err != nil {
		if gc != nil {
			gc()
		}
		return err
	}
	// defer setting `nerdctl/error` label in case of error
	defer func() {
		if err != nil {
			containerutil.UpdateErrorLabel(ctx, container, err)
		}
	}()

	fmt.Fprintln(cmd.OutOrStdout(), container.ID())
	return nil
}
