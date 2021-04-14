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

var composeLogsCommand = &cli.Command{
	Name:   "logs",
	Usage:  "View output from containers.",
	Action: composeLogsAction,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "follow",
			Aliases: []string{"f"},
			Usage:   "Follow log output.",
		},
		&cli.BoolFlag{
			Name:  "no-color",
			Usage: "Produce monochrome output",
		},
		&cli.BoolFlag{
			Name:  "no-log-prefix",
			Usage: "Don't print prefix in logs",
		},
	},
}

func composeLogsAction(clicontext *cli.Context) error {
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
	lo := composer.LogsOptions{
		Follow:      clicontext.Bool("follow"),
		NoColor:     clicontext.Bool("no-color"),
		NoLogPrefix: clicontext.Bool("no-log-prefix"),
	}
	return c.Logs(ctx, lo)
}
