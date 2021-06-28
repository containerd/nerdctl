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
	"fmt"

	"github.com/containerd/containerd/identifiers"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

var volumeCreateCommand = &cli.Command{
	Name:      "create",
	Usage:     "Create a volume",
	ArgsUsage: "[flags] VOLUME",
	Action:    volumeCreateAction,
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:  "label",
			Usage: "Set metadata for a volume",
		},
	},
}

func volumeCreateAction(clicontext *cli.Context) error {
	if clicontext.NArg() != 1 {
		return errors.Errorf("requires exactly 1 argument")
	}
	name := clicontext.Args().First()
	if err := identifiers.Validate(name); err != nil {
		return errors.Wrapf(err, "malformed name %s", name)
	}

	volStore, err := getVolumeStore(clicontext)
	if err != nil {
		return err
	}
	labels := strutil.DedupeStrSlice(clicontext.StringSlice("label"))
	if _, err := volStore.Create(name, labels); err != nil {
		return err
	}
	fmt.Fprintf(clicontext.App.Writer, "%s\n", name)
	return nil
}
