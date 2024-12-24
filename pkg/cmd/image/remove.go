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

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/log"
	"github.com/containerd/platforms"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/containerutil"
	"github.com/containerd/nerdctl/v2/pkg/idutil/imagewalker"
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
			if found.NameMatchIndex == -1 {
				// if found multiple images, return error unless in force-mode and
				// there is only 1 unique image.
				if found.MatchCount > 1 && !(options.Force && found.UniqueImages == 1) {
					return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
				}
			} else if found.NameMatchIndex != found.MatchIndex {
				// when there is an image with a name matching the argument but the argument is a digest short id,
				// the deletion process is not performed.
				return nil
			}

			if cid, ok := runningImages[found.Image.Name]; ok {
				if options.Force {
					if err = is.Delete(ctx, found.Image.Name); err != nil {
						return err
					}
					fmt.Fprintf(options.Stdout, "Untagged: %s\n", found.Image.Name)
					fmt.Fprintf(options.Stdout, "Untagged: %s\n", found.Image.Target.Digest.String())

					found.Image.Name = ":"
					if _, err = is.Create(ctx, found.Image); err != nil {
						return err
					}
					return nil
				}
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
		OnFoundCriRm: func(ctx context.Context, found imagewalker.Found) (bool, error) {
			if found.NameMatchIndex == -1 {
				// if found multiple images, return error unless in force-mode and
				// there is only 1 unique image.
				if found.MatchCount > 1 && !(options.Force && found.UniqueImages == 1) {
					return false, fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
				}
			} else if found.NameMatchIndex != found.MatchIndex {
				// when there is an image with a name matching the argument but the argument is a digest short id,
				// the deletion process is not performed.
				return false, nil
			}

			if cid, ok := runningImages[found.Image.Name]; ok {
				if options.Force {
					if err = is.Delete(ctx, found.Image.Name); err != nil {
						return false, err
					}
					fmt.Fprintf(options.Stdout, "Untagged: %s\n", found.Image.Name)
					fmt.Fprintf(options.Stdout, "Untagged: %s\n", found.Image.Target.Digest.String())

					found.Image.Name = ":"
					if _, err = is.Create(ctx, found.Image); err != nil {
						return false, err
					}
					return false, nil
				}
				return false, fmt.Errorf("conflict: unable to delete %s (cannot be forced) - image is being used by running container %s", found.Req, cid)
			}
			if cid, ok := usedImages[found.Image.Name]; ok && !options.Force {
				return false, fmt.Errorf("conflict: unable to delete %s (must be forced) - image is being used by stopped container %s", found.Req, cid)
			}
			// digests is used only for emulating human-readable output of `docker rmi`
			digests, err := found.Image.RootFS(ctx, cs, platforms.DefaultStrict())
			if err != nil {
				log.G(ctx).WithError(err).Warning("failed to enumerate rootfs")
			}

			if err := is.Delete(ctx, found.Image.Name, delOpts...); err != nil {
				return false, err
			}
			fmt.Fprintf(options.Stdout, "Untagged: %s@%s\n", found.Image.Name, found.Image.Target.Digest)
			for _, digest := range digests {
				fmt.Fprintf(options.Stdout, "Deleted: %s\n", digest)
			}
			return true, nil
		},
	}

	var errs []string
	var fatalErr bool
	for _, req := range args {
		var n int
		if options.GOptions.KubeHideDupe && options.GOptions.Namespace == "k8s.io" {
			n, err = walker.WalkCriRm(ctx, req)
		} else {
			n, err = walker.Walk(ctx, req)
		}
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
