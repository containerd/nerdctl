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
	"github.com/containerd/nerdctl/pkg/composer"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

var composeBuildCommand = &cli.Command{
	Name:   "build",
	Usage:  "Build or rebuild services",
	Action: composeBuildAction,
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:  "build-arg",
			Usage: "Set build-time variables for services.",
		},
		&cli.BoolFlag{
			Name:  "no-cache",
			Usage: "Do not use cache when building the image.",
		},
		&cli.StringFlag{
			Name:  "progress",
			Usage: "Set type of progress output",
		},
	},
}

func composeBuildAction(clicontext *cli.Context) error {
	if clicontext.NArg() != 0 {
		// TODO: support specifying service names as args
		return errors.Errorf("arguments %v not supported", clicontext.Args())
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	c, err := getComposer(clicontext, client)
	if err != nil {
		return err
	}
	bo := composer.BuildOptions{
		Args:     clicontext.StringSlice("build-arg"),
		NoCache:  clicontext.Bool("no-cache"),
		Progress: clicontext.String("progress"),
	}
	return c.Build(ctx, bo)
}
