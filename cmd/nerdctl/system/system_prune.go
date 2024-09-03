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

package system

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/builder"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/network"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/system"
)

func newSystemPruneCommand() *cobra.Command {
	systemPruneCommand := &cobra.Command{
		Use:           "prune [flags]",
		Short:         "Remove unused data",
		Args:          cobra.NoArgs,
		RunE:          systemPruneAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	systemPruneCommand.Flags().BoolP("all", "a", false, "Remove all unused images, not just dangling ones")
	systemPruneCommand.Flags().BoolP("force", "f", false, "Do not prompt for confirmation")
	systemPruneCommand.Flags().Bool("volumes", false, "Prune volumes")
	return systemPruneCommand
}

func processSystemPruneOptions(cmd *cobra.Command) (types.SystemPruneOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.SystemPruneOptions{}, err
	}

	all, err := cmd.Flags().GetBool("all")
	if err != nil {
		return types.SystemPruneOptions{}, err
	}

	vFlag, err := cmd.Flags().GetBool("volumes")
	if err != nil {
		return types.SystemPruneOptions{}, err
	}

	buildkitHost, err := builder.GetBuildkitHost(cmd, globalOptions.Namespace)
	if err != nil {
		log.L.WithError(err).Warn("BuildKit is not running. Build caches will not be pruned.")
		buildkitHost = ""
	}

	return types.SystemPruneOptions{
		Stdout:               cmd.OutOrStdout(),
		Stderr:               cmd.ErrOrStderr(),
		GOptions:             globalOptions,
		All:                  all,
		Volumes:              vFlag,
		BuildKitHost:         buildkitHost,
		NetworkDriversToKeep: network.NetworkDriversToKeep,
	}, nil
}

func grantSystemPrunePermission(cmd *cobra.Command, options types.SystemPruneOptions) (bool, error) {
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return false, err
	}

	if !force {
		var confirm string
		msg := `This will remove:
  - all stopped containers
  - all networks not used by at least one container`
		if options.Volumes {
			msg += `
  - all volumes not used by at least one container`
		}
		if options.All {
			msg += `
  - all images without at least one container associated to them
  - all build cache`
		} else {
			msg += `
  - all dangling images
  - all dangling build cache`
		}

		msg += "\nAre you sure you want to continue? [y/N] "
		fmt.Fprintf(options.Stdout, "WARNING! %s", msg)
		fmt.Fscanf(cmd.InOrStdin(), "%s", &confirm)

		if strings.ToLower(confirm) != "y" {
			return false, nil
		}
	}

	return true, nil
}

func systemPruneAction(cmd *cobra.Command, _ []string) error {
	options, err := processSystemPruneOptions(cmd)
	if err != nil {
		return err
	}

	if ok, err := grantSystemPrunePermission(cmd, options); err != nil {
		return err
	} else if !ok {
		return nil
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return system.Prune(ctx, client, options)
}
