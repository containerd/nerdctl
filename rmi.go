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
	"fmt"

	"github.com/containerd/containerd/platforms"
	"github.com/containerd/nerdctl/pkg/idutil/imagewalker"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

var rmiCommand = &cli.Command{
	Name:         "rmi",
	Usage:        "Remove one or more images",
	ArgsUsage:    "[flags] IMAGE [IMAGE, ...]",
	BashComplete: rmiBashComplete,
	Action:       rmiAction,
}

func rmiAction(clicontext *cli.Context) error {
	if clicontext.NArg() == 0 {
		return errors.Errorf("requires at least 1 argument")
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	cs := client.ContentStore()
	is := client.ImageService()

	walker := &imagewalker.ImageWalker{
		Client: client,
		OnFound: func(ctx context.Context, found imagewalker.Found) error {
			digests, err := found.Image.RootFS(ctx, cs, platforms.DefaultStrict())
			if err != nil {
				return err
			}

			if err := is.Delete(ctx, found.Image.Name); err != nil {
				return err
			}
			fmt.Fprintf(clicontext.App.Writer, "Untagged: %s@%s\n", found.Image.Name, found.Image.Target.Digest)
			for _, digest := range digests {
				fmt.Fprintf(clicontext.App.Writer, "Deleted: %s\n", digest)
			}
			return nil
		},
	}
	for _, req := range clicontext.Args().Slice() {
		n, err := walker.Walk(ctx, req)
		if err != nil {
			return err
		} else if n == 0 {
			return errors.Errorf("no such image %s", req)
		}
	}
	return nil
}

func rmiBashComplete(clicontext *cli.Context) {
	coco := parseCompletionContext(clicontext)
	if coco.boring || coco.flagTakesValue {
		defaultBashComplete(clicontext)
		return
	}
	// show image names
	bashCompleteImageNames(clicontext)
}
