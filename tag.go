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
	"github.com/containerd/containerd/errdefs"
	refdocker "github.com/containerd/containerd/reference/docker"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

var tagCommand = &cli.Command{
	Name:         "tag",
	Usage:        "Create a tag TARGET_IMAGE that refers to SOURCE_IMAGE",
	ArgsUsage:    "SOURCE_IMAGE[:TAG] TARGET_IMAGE[:TAG]",
	Action:       tagAction,
	BashComplete: tagBashComplete,
}

func tagAction(clicontext *cli.Context) error {
	if clicontext.NArg() != 2 {
		return errors.Errorf("requires exactly 2 arguments")
	}

	src, err := refdocker.ParseDockerRef(clicontext.Args().Get(0))
	if err != nil {
		return err
	}

	target, err := refdocker.ParseDockerRef(clicontext.Args().Get(1))
	if err != nil {
		return err
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	ctx, done, err := client.WithLease(ctx)
	if err != nil {
		return err
	}
	defer done(ctx)

	imageService := client.ImageService()
	image, err := imageService.Get(ctx, src.String())
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

func tagBashComplete(clicontext *cli.Context) {
	coco := parseCompletionContext(clicontext)
	if coco.boring || coco.flagTakesValue {
		defaultBashComplete(clicontext)
		return
	}
	// show image names
	bashCompleteImageNames(clicontext)
}
