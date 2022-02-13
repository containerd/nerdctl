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

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/namespaces"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newContainerPruneCommand() *cobra.Command {
	containerPruneCommand := &cobra.Command{
		Use:           "prune [flags]",
		Short:         "Remove all stopped containers",
		RunE:          containerPruneAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	containerPruneCommand.Flags().BoolP("force", "f", false, "Ignore removal errors")
	return containerPruneCommand
}

func containerPruneAction(cmd *cobra.Command, _ []string) error {
	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}

	ns, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	}

	ctx = namespaces.WithNamespace(ctx, ns)

	containers, err := client.Containers(ctx)
	if err != nil {
		return err
	}

	for _, container := range containers {
		task, err := container.Task(ctx, cio.Load)
		if err != nil {
			return err
		}

		status, err := task.Status(ctx)
		if err != nil {
			return err
		}

		if status.Status == containerd.Stopped {
			if _, err := task.Delete(ctx); err != nil && !errdefs.IsNotFound(err) {
				return fmt.Errorf("failed to delete task %v: %w", task.ID(), err)
			}
			if err := container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
				err = container.Delete(ctx)
				if err != nil {
					if force {
						logrus.WithError(err).WithField("id", container.ID()).Warn("unable to remove container")
					} else {
						return err
					}
				}
			}
		}
	}

	return nil
}
