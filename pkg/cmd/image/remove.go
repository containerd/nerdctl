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
	"errors"
	"fmt"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/images"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/containerutil"
	"github.com/containerd/nerdctl/pkg/idutil/imagewalker"
	"github.com/containerd/platforms"
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
	usedImages := make(map[string]string)
	runningImages := make(map[string]string)
	for _, container := range containerList {
		image, err := container.Image(ctx)
		if err != nil {
			continue
		}

		// if err != nil, simply go to `default`
		switch cStatus, _ := containerutil.ContainerStatus(ctx, container); cStatus.Status {
		case containerd.Running, containerd.Pausing, containerd.Paused:
			runningImages[image.Name()] = container.ID()
		default:
			usedImages[image.Name()] = container.ID()
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
			if cid, ok := runningImages[found.Image.Name]; ok {
				return fmt.Errorf("conflict: unable to delete %s (cannot be forced) - image is being used by running container %s", found.Req, cid)
			}
			if cid, ok := usedImages[found.Image.Name]; ok && !options.Force {
				return fmt.Errorf("conflict: unable to delete %s (must be forced) - image is being used by stopped container %s", found.Req, cid)
			}
			// digests is used only for emulating human-readable output of `docker rmi`
			digests, err := found.Image.RootFS(ctx, cs, platforms.DefaultStrict())
			if err != nil {
				log.G(ctx).WithError(err).Warning("failed to enumerate rootfs")
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

	var errs []string
	var fatalErr bool
	for _, req := range args {
		n, err := walker.Walk(ctx, req)
		if err != nil {
			fatalErr = true
		}
		if err == nil && n == 0 {
			err = fmt.Errorf("no such image: %s", req)
		}
		if err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) > 0 {
		msg := fmt.Sprintf("%d errors:\n%s", len(errs), strings.Join(errs, "\n"))
		if !options.Force || fatalErr {
			return errors.New(msg)
		}
		log.G(ctx).Error(msg)
	}
	return nil
}
