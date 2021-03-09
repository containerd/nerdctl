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
	"encoding/json"
	"fmt"
	"time"

	"github.com/AkihiroSuda/nerdctl/pkg/containerinspector"
	"github.com/AkihiroSuda/nerdctl/pkg/idutil/containerwalker"
	"github.com/AkihiroSuda/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

var containerInspectCommand = &cli.Command{
	Name:         "inspect",
	Usage:        "Display detailed information on one or more containers.",
	ArgsUsage:    "[flags] CONTAINER [CONTAINER, ...]",
	Description:  "Hint: set `--mode=native` for showing the full output",
	Action:       containerInspectAction,
	BashComplete: containerInspectBashComplete,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "mode",
			Usage: "Inspect mode, \"dockercompat\" for Docker-compatible output, \"native\" for containerd-native output",
			Value: "dockercompat",
		},
	},
}

func containerInspectAction(clicontext *cli.Context) error {
	if clicontext.NArg() == 0 {
		return errors.Errorf("requires at least 1 argument")
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	f := &containerInspector{
		mode: clicontext.String("mode"),
	}
	walker := &containerwalker.ContainerWalker{
		Client:  client,
		OnFound: f.Handler,
	}

	var errs []error
	for _, req := range clicontext.Args().Slice() {
		n, err := walker.Walk(ctx, req)
		if err != nil {
			errs = append(errs, err)
		} else if n == 0 {
			errs = append(errs, errors.Errorf("no such object: %s", req))
		}
	}

	b, err := json.MarshalIndent(f.entries, "", "    ")
	if err != nil {
		return err
	}
	fmt.Fprintln(clicontext.App.Writer, string(b))

	if len(errs) > 0 {
		return errors.Errorf("%d errors: %+v", len(errs), errs)
	}
	return nil
}

type containerInspector struct {
	mode    string
	entries []interface{}
}

func (x *containerInspector) Handler(ctx context.Context, found containerwalker.Found) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	n, err := containerinspector.Inspect(ctx, found.Container)
	if err != nil {
		return err
	}
	switch x.mode {
	case "native":
		x.entries = append(x.entries, n)
	case "dockercompat":
		d, err := dockercompat.ContainerFromNative(n)
		if err != nil {
			return err
		}
		x.entries = append(x.entries, d)
	default:
		return errors.Errorf("unknown mode %q", x.mode)
	}
	return nil
}

func containerInspectBashComplete(clicontext *cli.Context) {
	if _, ok := isFlagCompletionContext(); ok {
		defaultBashComplete(clicontext)
		return
	}
	// show container names
	bashCompleteContainerNames(clicontext)
}
