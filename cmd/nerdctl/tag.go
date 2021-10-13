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

	"github.com/containerd/containerd/errdefs"
	refdocker "github.com/containerd/containerd/reference/docker"
	"github.com/containerd/nerdctl/pkg/idutil/imagewalker"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func newTagCommand() *cobra.Command {
	var tagCommand = &cobra.Command{
		Use:               "tag SOURCE_IMAGE[:TAG] TARGET_IMAGE[:TAG]",
		Short:             "Create a tag TARGET_IMAGE that refers to SOURCE_IMAGE",
		Args:              cobra.ExactArgs(2),
		RunE:              tagAction,
		ValidArgsFunction: tagShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	return tagCommand
}

func tagAction(cmd *cobra.Command, args []string) error {
	if len(args) != 2 {
		return errors.Errorf("requires exactly 2 arguments")
	}

	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	imageService := client.ImageService()
	var srcName string
	imagewalker := &imagewalker.ImageWalker{
		Client: client,
		OnFound: func(ctx context.Context, found imagewalker.Found) error {
			if srcName == "" {
				srcName = found.Image.Name
			}
			return nil
		},
	}
	matchCount, err := imagewalker.Walk(ctx, args[0])
	if err != nil {
		return err
	}
	if matchCount < 1 {
		return errors.Errorf("%s: not found", args[0])
	}

	target, err := refdocker.ParseDockerRef(args[1])
	if err != nil {
		return err
	}

	ctx, done, err := client.WithLease(ctx)
	if err != nil {
		return err
	}
	defer done(ctx)

	image, err := imageService.Get(ctx, srcName)
	if err != nil {
		return err
	}
	image.Name = target.String()
	if _, err = imageService.Create(ctx, image); err != nil {
		if errdefs.IsAlreadyExists(err) {
			if err = imageService.Delete(ctx, image.Name); err != nil {
				return err
			}
			if _, err = imageService.Create(ctx, image); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	return nil
}

func tagShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) < 2 {
		// show image names
		return shellCompleteImageNames(cmd)
	} else {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}
