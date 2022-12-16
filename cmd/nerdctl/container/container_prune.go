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
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/containerd/containerd"
	ncclient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/common"
	containerUtils "github.com/containerd/nerdctl/cmd/nerdctl/utils/container"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func NewPruneCommand() *cobra.Command {
	containerPruneCommand := &cobra.Command{
		Use:           "prune [flags]",
		Short:         "Remove all stopped containers",
		Args:          cobra.NoArgs,
		RunE:          containerPruneAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	containerPruneCommand.Flags().BoolP("force", "f", false, "Do not prompt for confirmation")
	return containerPruneCommand
}

func containerPruneAction(cmd *cobra.Command, _ []string) error {
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}

	if !force {
		var confirm string
		msg := "This will remove all stopped containers."
		msg += "\nAre you sure you want to continue? [y/N] "
		fmt.Fprintf(cmd.OutOrStdout(), "WARNING! %s", msg)
		fmt.Fscanf(cmd.InOrStdin(), "%s", &confirm)

		if strings.ToLower(confirm) != "y" {
			return nil
		}
	}

	client, ctx, cancel, err := ncclient.NewClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	return Prune(ctx, cmd, client)
}

func Prune(ctx context.Context, cmd *cobra.Command, client *containerd.Client) error {
	containers, err := client.Containers(ctx)
	if err != nil {
		return err
	}

	var deleted []string
	for _, container := range containers {
		err = containerUtils.RemoveContainer(ctx, cmd, container, false, true)
		if err == nil {
			deleted = append(deleted, container.ID())
			continue
		}
		if errors.As(err, &common.StatusError{}) {
			continue
		}
		logrus.WithError(err).Warnf("failed to remove container %s", container.ID())
	}

	if len(deleted) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Deleted Containers:")
		for _, id := range deleted {
			fmt.Fprintln(cmd.OutOrStdout(), id)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "")
	}

	return nil
}
