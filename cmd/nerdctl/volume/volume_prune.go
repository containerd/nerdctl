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

package volume

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/volume"
)

func newVolumePruneCommand() *cobra.Command {
	volumePruneCommand := &cobra.Command{
		Use:           "prune [flags]",
		Short:         "Remove all unused local volumes",
		Args:          cobra.NoArgs,
		RunE:          volumePruneAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	volumePruneCommand.Flags().BoolP("all", "a", false, "Remove all unused volumes, not just anonymous ones")
	volumePruneCommand.Flags().BoolP("force", "f", false, "Do not prompt for confirmation")
	return volumePruneCommand
}

func processVolumePruneOptions(cmd *cobra.Command) (types.VolumePruneOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.VolumePruneOptions{}, err
	}

	all, err := cmd.Flags().GetBool("all")
	if err != nil {
		return types.VolumePruneOptions{}, err
	}

	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return types.VolumePruneOptions{}, err
	}

	options := types.VolumePruneOptions{
		GOptions: globalOptions,
		All:      all,
		Force:    force,
		Stdout:   cmd.OutOrStdout(),
	}
	return options, nil
}

func volumePruneAction(cmd *cobra.Command, _ []string) error {
	options, err := processVolumePruneOptions(cmd)
	if err != nil {
		return err
	}

	if !options.Force {
		var confirm string
		msg := "This will remove all local volumes not used by at least one container."
		msg += "\nAre you sure you want to continue? [y/N] "
		fmt.Fprintf(options.Stdout, "WARNING! %s", msg)
		fmt.Fscanf(cmd.InOrStdin(), "%s", &confirm)

		if strings.ToLower(confirm) != "y" {
			return nil
		}
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return volume.Prune(ctx, client, options)
}
