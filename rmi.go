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
	"fmt"

	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/nerdctl/pkg/idutil/imagewalker"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newRmiCommand() *cobra.Command {
	var rmiCommand = &cobra.Command{
		Use:               "rmi [flags] IMAGE [IMAGE, ...]",
		Short:             "Remove one or more images",
		Args:              cobra.MinimumNArgs(1),
		RunE:              rmiAction,
		ValidArgsFunction: rmiShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	rmiCommand.Flags().BoolP("force", "f", false, "Ignore removal errors")
	// Alias `-a` is reserved for `--all`. Should be compatible with `podman rmi --all`.
	rmiCommand.Flags().Bool("async", false, "Asynchronous mode")
	return rmiCommand
}

func rmiAction(cmd *cobra.Command, args []string) error {
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}

	var delOpts []images.DeleteOpt
	if async, err := cmd.Flags().GetBool("async"); err != nil {
		return err
	} else if !async {
		delOpts = append(delOpts, images.SynchronousDelete())
	}

	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	cs := client.ContentStore()
	is := client.ImageService()

	walker := &imagewalker.ImageWalker{
		Client: client,
		OnFound: func(ctx context.Context, found imagewalker.Found) error {
			// digests is used only for emulating human-readable output of `docker rmi`
			digests, err := found.Image.RootFS(ctx, cs, platforms.DefaultStrict())
			if err != nil {
				logrus.WithError(err).Warning("failed to enumerate rootfs")
			}

			if err := is.Delete(ctx, found.Image.Name, delOpts...); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Untagged: %s@%s\n", found.Image.Name, found.Image.Target.Digest)
			for _, digest := range digests {
				fmt.Fprintf(cmd.OutOrStdout(), "Deleted: %s\n", digest)
			}
			return nil
		},
	}
	for _, req := range args {
		n, err := walker.Walk(ctx, req)
		if err == nil && n == 0 {
			err = fmt.Errorf("no such image %s", req)
		}
		if err != nil {
			if force {
				logrus.Error(err)
			} else {
				return err
			}
		}
	}
	return nil
}

func rmiShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show image names
	return shellCompleteImageNames(cmd)
}
