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
	"github.com/urfave/cli/v2"
)

var composeUpCommand = &cli.Command{
	Name:   "up",
	Usage:  "Create and start containers",
	Action: composeUpAction,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "detach",
			Aliases: []string{"d"},
			Usage:   "Detached mode: Run containers in the background",
		},
		&cli.BoolFlag{
			Name:  "no-color",
			Usage: "Produce monochrome output",
		},
		&cli.BoolFlag{
			Name:  "no-log-prefix",
			Usage: "Don't print prefix in logs",
		},
		&cli.BoolFlag{
			Name:  "build",
			Usage: "Build images before starting containers.",
		},
	},
}

func composeUpAction(clicontext *cli.Context) error {
	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	c, err := getComposer(clicontext, client)
	if err != nil {
		return err
	}
	uo := composer.UpOptions{
		Detach:      clicontext.Bool("detach"),
		NoColor:     clicontext.Bool("no-color"),
		NoLogPrefix: clicontext.Bool("no-log-prefix"),
		ForceBuild:  clicontext.Bool("build"),
	}
	return c.Up(ctx, uo)
}
