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

	"github.com/AkihiroSuda/nerdctl/pkg/idutil/containerwalker"
	"github.com/AkihiroSuda/nerdctl/pkg/imgutil/commit"
	refdocker "github.com/containerd/containerd/reference/docker"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

var (
	commitCommand = &cli.Command{
		Name:        "commit",
		Usage:       "[flags] CONTAINER REPOSITORY[:TAG]",
		Description: "Create a new image from a container's changes",
		Action:      commitAction,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "author",
				Aliases: []string{"a"},
				Usage:   `Author (e.g., "nerdctl contributor <nerdctl-dev@example.com>")`,
			},
			&cli.StringFlag{
				Name:    "message",
				Aliases: []string{"m"},
				Usage:   `Commit message`,
			},
		},
	}
)

func commitAction(clicontext *cli.Context) error {
	if clicontext.NArg() != 2 {
		return errors.New("need container and commit image name")
	}

	opts, err := newCommitOpts(clicontext)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return errors.Errorf("ambiguous ID %q", found.Req)
			}
			imageID, err := commit.Commit(ctx, client, found.Container.ID(), opts)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(clicontext.App.Writer, "%s\n", imageID)
			return err
		},
	}
	req := clicontext.Args().First()
	n, err := walker.Walk(ctx, req)
	if err != nil {
		return err
	} else if n == 0 {
		return errors.Errorf("no such container %s", req)
	}
	return nil
}

func newCommitOpts(clicontext *cli.Context) (*commit.Opts, error) {
	rawRef := clicontext.Args().Get(1)

	named, err := refdocker.ParseDockerRef(rawRef)
	if err != nil {
		return nil, err
	}

	return &commit.Opts{
		Author:  clicontext.String("author"),
		Message: clicontext.String("message"),
		Ref:     named.String(),
	}, nil
}
