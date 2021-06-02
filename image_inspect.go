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
	"encoding/json"
	"fmt"
	"time"

	"github.com/containerd/nerdctl/pkg/idutil/imagewalker"
	"github.com/containerd/nerdctl/pkg/imageinspector"
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

var imageInspectCommand = &cli.Command{
	Name:         "inspect",
	Usage:        "Display detailed information on one or more images.",
	ArgsUsage:    "[OPTIONS] IMAGE [IMAGE...]",
	Description:  "Hint: set `--mode=native` for showing the full output",
	Action:       ImageInspectAction,
	BashComplete: imageInspectBashComplete,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "mode",
			Usage: "Inspect mode, \"dockercompat\" for Docker-compatible output, \"native\" for containerd-native output",
			Value: "dockercompat",
		},
	},
}

func ImageInspectAction(clicontext *cli.Context) error {
	if clicontext.NArg() == 0 {
		return errors.Errorf("requires at least 1 argument")
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	f := &imageInspector{
		mode: clicontext.String("mode"),
	}
	walker := &imagewalker.ImageWalker{
		Client: client,
		OnFound: func(ctx context.Context, found imagewalker.Found) error {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			n, err := imageinspector.Inspect(ctx, client, found.Image)
			if err != nil {
				return err
			}
			switch f.mode {
			case "native":
				f.entries = append(f.entries, n)
			case "dockercompat":
				d, err := dockercompat.ImageFromNative(n)
				if err != nil {
					return err
				}
				f.entries = append(f.entries, d)
			default:
				return errors.Errorf("unknown mode %q", f.mode)
			}
			return nil
		},
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
		return errors.Errorf("%d errors: %v", len(errs), errs)
	}
	return nil
}

type imageInspector struct {
	mode    string
	entries []interface{}
}

func imageInspectBashComplete(clicontext *cli.Context) {
	coco := parseCompletionContext(clicontext)
	if coco.boring {
		defaultBashComplete(clicontext)
		return
	}
	if coco.flagTakesValue {
		w := clicontext.App.Writer
		switch coco.flagName {
		case "mode":
			fmt.Fprintln(w, "dockercompat")
			fmt.Fprintln(w, "native")
			return
		}
		defaultBashComplete(clicontext)
		return
	}
	// show image names
	bashCompleteImageNames(clicontext)
}
