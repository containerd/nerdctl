/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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
	"io"

	"github.com/containerd/containerd"
	"github.com/pkg/errors"

	"github.com/containerd/containerd/cmd/ctr/commands/content"
	"github.com/containerd/containerd/platforms"
	refdocker "github.com/containerd/containerd/reference/docker"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/urfave/cli/v2"
)

var pullCommand = &cli.Command{
	Name:   "pull",
	Usage:  "Pull an image from a registry",
	Action: pullAction,
	Flags:  []cli.Flag{},
}

func pullAction(clicontext *cli.Context) error {
	if clicontext.NArg() < 1 {
		return errors.New("image name needs to be specified")
	}
	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()
	_, err = ensureImage(ctx, client, clicontext.App.Writer, clicontext.Args().First(), "always")
	return err
}

type EnsuredImage struct {
	Ref         string
	Image       containerd.Image
	Snapshotter string
}

// PullMode is either one of "always", "missing", "never"
type PullMode = string

func ensureImage(ctx context.Context, client *containerd.Client, stdout io.Writer, rawRef string, mode PullMode) (*EnsuredImage, error) {
	named, err := refdocker.ParseDockerRef(rawRef)
	if err != nil {
		return nil, err
	}
	ref := named.String()

	sn := containerd.DefaultSnapshotter
	if mode != "always" {
		if i, err := client.ImageService().Get(ctx, ref); err == nil {
			image := containerd.NewImage(client, i)
			res := &EnsuredImage{
				Ref:         ref,
				Image:       image,
				Snapshotter: sn,
			}
			if unpacked, err := image.IsUnpacked(ctx, sn); err == nil && !unpacked {
				if err := image.Unpack(ctx, sn); err != nil {
					return nil, err
				}
			}
			return res, nil
		}
	}

	if mode == "never" {
		return nil, errors.Errorf("image %q is not available", rawRef)
	}

	ctx, done, err := client.WithLease(ctx)
	if err != nil {
		return nil, err
	}
	defer done(ctx)

	resovlerOpts := docker.ResolverOptions{}
	resolver := docker.NewResolver(resovlerOpts)

	config := &content.FetchConfig{
		Resolver:        resolver,
		ProgressOutput:  stdout,
		PlatformMatcher: platforms.Default(),
	}

	img, err := content.Fetch(ctx, client, ref, config)
	if err != nil {
		return nil, err
	}
	i := containerd.NewImageWithPlatform(client, img, config.PlatformMatcher)
	if err = i.Unpack(ctx, sn); err != nil {
		return nil, err
	}
	res := &EnsuredImage{
		Ref:         ref,
		Image:       i,
		Snapshotter: sn,
	}
	return res, nil
}
