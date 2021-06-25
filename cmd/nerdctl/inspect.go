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

	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/idutil/imagewalker"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

var inspectCommand = &cli.Command{
	Name:         "inspect",
	Usage:        "Return low-level information on objects.",
	Description:  containerInspectCommand.Description,
	Action:       inspectAction,
	BashComplete: containerInspectBashComplete,
	Flags:        containerInspectCommand.Flags,
}

func inspectAction(clicontext *cli.Context) error {
	if clicontext.NArg() == 0 {
		return errors.Errorf("requires at least 1 argument")
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	imagewalker := &imagewalker.ImageWalker{
		Client: client,
		OnFound: func(ctx context.Context, found imagewalker.Found) error {
			return nil
		},
	}

	containerwalker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			return nil
		},
	}

	for _, req := range clicontext.Args().Slice() {
		ni, err := imagewalker.Walk(ctx, req)
		if err != nil {
			return err
		}

		nc, err := containerwalker.Walk(ctx, req)
		if err != nil {
			return err
		}

		if ni != 0 && nc != 0 {
			return errors.Errorf("multiple IDs found with provided prefix: %s", req)
		}

		if nc == 0 && ni == 0 {
			return errors.Errorf("no such object %s", req)
		}
		if ni != 0 {
			if err := ImageInspectAction(clicontext); err != nil {
				return err
			}
		} else {
			if err := ContainerInspectAction(clicontext); err != nil {
				return err
			}
		}
	}

	return nil
}
