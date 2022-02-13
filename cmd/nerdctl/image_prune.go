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

	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/platforms"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newImagePruneCommand() *cobra.Command {
	imagePruneCommand := &cobra.Command{
		Use:           "prune [flags]",
		Short:         "Remove all unused images",
		RunE:          imagePruneAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	imagePruneCommand.Flags().BoolP("force", "f", false, "Ignore removal errors")
	return imagePruneCommand
}

func imagePruneAction(cmd *cobra.Command, _ []string) error {
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

	imageService := client.ImageService()
	contentStore := client.ContentStore()
	containerService := client.ContainerService()

	images, err := imageService.List(ctx)
	if err != nil {
		return err
	}

	containers, err := containerService.List(ctx)
	if err != nil {
		return err
	}

	for _, image := range images {
		used := false
		for _, container := range containers {
			if container.Image == image.Name {
				used = true
			}
		}

		if used {
			continue
		}

		digests, err := image.RootFS(ctx, contentStore, platforms.All)
		if err != nil {
			return err
		}

		err = imageService.Delete(ctx, image.Name)
		if err != nil {
			if force {
				logrus.WithError(err).Error("unable to remove image")
			} else {
				return err
			}
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Untagged: %s@%s\n", image.Name, image.Target.Digest)
			for _, digest := range digests {
				fmt.Fprintf(cmd.OutOrStdout(), "Deleted: %s\n", digest)
			}
		}
	}

	return nil
}
