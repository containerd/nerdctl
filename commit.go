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

	"github.com/AkihiroSuda/nerdctl/pkg/idutil"
	"github.com/AkihiroSuda/nerdctl/pkg/imgutil/commit"
	"github.com/containerd/containerd"
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

	return idutil.WalkContainers(ctx, client, []string{clicontext.Args().First()}, func(ctx context.Context, client *containerd.Client, _, id string) error {
		imageID, err := commit.Commit(ctx, client, id, opts)
		if err != nil {
			return err
		}

		_, err = fmt.Fprintf(clicontext.App.Writer, "%s\n", imageID)
		return err
	})
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
