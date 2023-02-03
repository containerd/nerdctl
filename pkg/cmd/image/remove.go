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

package image

import (
	"context"
	"fmt"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/idutil/imagewalker"
	"github.com/sirupsen/logrus"
)

// Remove removes a list of `images`.
func Remove(ctx context.Context, client *containerd.Client, args []string, options types.ImageRemoveOptions) error {
	var delOpts []images.DeleteOpt
	if !options.Async {
		delOpts = append(delOpts, images.SynchronousDelete())
	}

	cs := client.ContentStore()
	is := client.ImageService()
	containerList, err := client.Containers(ctx)
	if err != nil {
		return err
	}
	usedImages := make(map[string]struct{})
	runningImages := make(map[string]struct{})
	for _, container := range containerList {
		image, err := container.Image(ctx)
		if err != nil {
			return err
		}
		cStatus := formatter.ContainerStatus(ctx, container)
		if strings.HasPrefix(cStatus, "Up") {
			runningImages[image.Name()] = struct{}{}
		} else {
			usedImages[image.Name()] = struct{}{}
		}
	}

	walker := &imagewalker.ImageWalker{
		Client: client,
		OnFound: func(ctx context.Context, found imagewalker.Found) error {
			// if found multiple images, return error unless in force-mode and
			// there is only 1 unique image.
			if found.MatchCount > 1 && !(options.Force && found.UniqueImages == 1) {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			if _, ok := runningImages[found.Image.Name]; ok {
				return fmt.Errorf("image %s is running, can't be forced removed", found.Image.Name)
			}
			if _, ok := usedImages[found.Image.Name]; ok && !options.Force {
				return fmt.Errorf("conflict: unable to remove repository reference %q (must force)", found.Req)
			}
			// digests is used only for emulating human-readable output of `docker rmi`
			digests, err := found.Image.RootFS(ctx, cs, platforms.DefaultStrict())
			if err != nil {
				logrus.WithError(err).Warning("failed to enumerate rootfs")
			}

			if err := is.Delete(ctx, found.Image.Name, delOpts...); err != nil {
				return err
			}
			fmt.Fprintf(options.Stdout, "Untagged: %s@%s\n", found.Image.Name, found.Image.Target.Digest)
			for _, digest := range digests {
				fmt.Fprintf(options.Stdout, "Deleted: %s\n", digest)
			}
			return nil
		},
	}

	err = walker.WalkAll(ctx, args, true)
	if err != nil && options.Force {
		logrus.Error(err)
		return nil
	}
	return err
}
