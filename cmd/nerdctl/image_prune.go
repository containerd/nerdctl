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
	"strings"

	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/platforms"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newImagePruneCommand() *cobra.Command {
	imagePruneCommand := &cobra.Command{
		Use:           "prune [flags]",
		Short:         "Remove unused images",
		Args:          cobra.NoArgs,
		RunE:          imagePruneAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	imagePruneCommand.Flags().BoolP("all", "a", false, "Remove all unused images, not just dangling ones")
	imagePruneCommand.Flags().BoolP("force", "f", false, "Do not prompt for confirmation")
	return imagePruneCommand
}

func imagePruneAction(cmd *cobra.Command, _ []string) error {
	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	all, err := cmd.Flags().GetBool("all")
	if err != nil {
		return err
	}

	if !all {
		logrus.Warn("Currently, `nerdctl image prune` requires --all to be specified. Skip pruning.")
		// NOP
		return nil
	}

	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}

	if !force {
		var confirm string
		msg := "This will remove all images without at least one container associated to them."
		msg += "\nAre you sure you want to continue? [y/N] "

		fmt.Fprintf(cmd.OutOrStdout(), "WARNING! %s", msg)
		fmt.Fscanf(cmd.InOrStdin(), "%s", &confirm)

		if strings.ToLower(confirm) != "y" {
			return nil
		}
	}
	var (
		imageStore     = client.ImageService()
		contentStore   = client.ContentStore()
		containerStore = client.ContainerService()
	)
	imageList, err := imageStore.List(ctx)
	if err != nil {
		return err
	}
	containerList, err := containerStore.List(ctx)
	if err != nil {
		return err
	}
	usedImages := make(map[string]struct{})
	for _, container := range containerList {
		usedImages[container.Image] = struct{}{}
	}

	delOpts := []images.DeleteOpt{images.SynchronousDelete()}
	for _, image := range imageList {
		if _, ok := usedImages[image.Name]; ok {
			continue
		}

		digests, err := image.RootFS(ctx, contentStore, platforms.DefaultStrict())
		if err != nil {
			logrus.WithError(err).Warnf("failed to enumerate rootfs")
		}
		if err := imageStore.Delete(ctx, image.Name, delOpts...); err != nil {
			logrus.WithError(err).Warnf("failed to delete image %s", image.Name)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Untagged: %s\n", image.Name)
		for _, digest := range digests {
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted: %s\n", digest)
		}
	}
	return nil
}
