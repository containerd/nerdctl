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

var composeDownCommand = &cli.Command{
	Name:   "down",
	Usage:  "Remove containers and associated resources",
	Action: composeDownAction,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "volumes",
			Aliases: []string{"v"},
			Usage:   "Remove named volumes declared in the `volumes` section of the Compose file and anonymous volumes attached to containers.",
		},
	},
}

func composeDownAction(clicontext *cli.Context) error {
	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	c, err := getComposer(clicontext, client)
	if err != nil {
		return err
	}
	downOpts := composer.DownOptions{
		RemoveVolumes: clicontext.Bool("v"),
	}
	return c.Down(ctx, downOpts)
}
