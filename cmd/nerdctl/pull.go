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
	"github.com/AkihiroSuda/nerdctl/pkg/imgutil"
	"github.com/pkg/errors"

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
	_, err = imgutil.EnsureImage(ctx, client, clicontext.App.Writer, clicontext.String("snapshotter"), clicontext.Args().First(), "always")
	return err
}
