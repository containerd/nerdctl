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
	"context"
	"fmt"
	"strings"

	"github.com/containerd/containerd"
	nerdClient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/volume"
	"github.com/spf13/cobra"
)

func NewPruneCommand() *cobra.Command {
	volumePruneCommand := &cobra.Command{
		Use:           "prune [flags]",
		Short:         "Remove all unused local volumes",
		Args:          cobra.NoArgs,
		RunE:          volumePruneAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	volumePruneCommand.Flags().BoolP("force", "f", false, "Do not prompt for confirmation")
	return volumePruneCommand
}

func volumePruneAction(cmd *cobra.Command, _ []string) error {
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}

	if !force {
		var confirm string
		msg := "This will remove all local volumes not used by at least one container."
		msg += "\nAre you sure you want to continue? [y/N] "
		fmt.Fprintf(cmd.OutOrStdout(), "WARNING! %s", msg)
		fmt.Fscanf(cmd.InOrStdin(), "%s", &confirm)

		if strings.ToLower(confirm) != "y" {
			return nil
		}
	}

	client, ctx, cancel, err := nerdClient.NewClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	return Prune(ctx, cmd, client)
}

func Prune(ctx context.Context, cmd *cobra.Command, client *containerd.Client) error {
	volStore, err := volume.GetVolumeStore(cmd)
	if err != nil {
		return err
	}
	volumes, err := volStore.List(false)
	if err != nil {
		return err
	}
	containers, err := client.Containers(ctx)
	if err != nil {
		return err
	}
	usedVolumes, err := usedVolumes(ctx, containers)
	if err != nil {
		return err
	}
	var removeNames []string // nolint: prealloc
	for _, volume := range volumes {
		if _, ok := usedVolumes[volume.Name]; ok {
			continue
		}
		removeNames = append(removeNames, volume.Name)
	}
	removedNames, err := volStore.Remove(removeNames)
	if err != nil {
		return err
	}
	if len(removedNames) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Deleted Volumes:")
		for _, name := range removedNames {
			fmt.Fprintln(cmd.OutOrStdout(), name)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "")
	}
	return nil
}
