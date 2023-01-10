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
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newContainerPruneCommand() *cobra.Command {
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
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
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

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return containerPrune(ctx, cmd, client, globalOptions)
}

func containerPrune(ctx context.Context, cmd *cobra.Command, client *containerd.Client, globalOptions types.GlobalCommandOptions) error {
	containers, err := client.Containers(ctx)
	if err != nil {
		return err
	}

	var deleted []string
	for _, container := range containers {
		err = removeContainer(ctx, cmd, globalOptions, container, false, true)
		if err == nil {
			deleted = append(deleted, container.ID())
			continue
		}
		if errors.As(err, &statusError{}) {
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
