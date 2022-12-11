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

package tag

import (
	"context"
	"fmt"

	"github.com/containerd/containerd/errdefs"
	nerdClient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils"
	"github.com/containerd/nerdctl/pkg/idutil/imagewalker"
	"github.com/containerd/nerdctl/pkg/referenceutil"

	"github.com/spf13/cobra"
)

func NewTagCommand() *cobra.Command {
	var tagCommand = &cobra.Command{
		Use:               "tag [flags] SOURCE_IMAGE[:TAG] TARGET_IMAGE[:TAG]",
		Short:             "Create a tag TARGET_IMAGE that refers to SOURCE_IMAGE",
		Args:              utils.IsExactArgs(2),
		RunE:              tagAction,
		ValidArgsFunction: tagShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	return tagCommand
}

func tagAction(cmd *cobra.Command, args []string) error {
	client, ctx, cancel, err := nerdClient.NewClient(cmd)
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
		return fmt.Errorf("%s: not found", args[0])
	}

	target, err := referenceutil.ParseDockerRef(args[1])
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
		return completion.ShellCompleteImageNames(cmd)
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}
