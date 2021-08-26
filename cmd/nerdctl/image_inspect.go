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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"text/template"
	"time"

	"github.com/containerd/nerdctl/pkg/idutil/imagewalker"
	"github.com/containerd/nerdctl/pkg/imageinspector"
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/docker/cli/templates"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
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
		&cli.StringFlag{
			Name:    "format",
			Aliases: []string{"f"},
			Usage:   "Format the output using the given Go template, e.g, '{{json .}}'",
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

	var tmpl *template.Template
	switch format := clicontext.String("format"); format {
	case "":
		b, err := json.MarshalIndent(f.entries, "", "    ")
		if err != nil {
			return err
		}
		fmt.Fprintln(clicontext.App.Writer, string(b))
	case "raw", "table":
		return errors.New("unsupported format: \"raw\" and \"table\"")
	default:
		var err error
		tmpl, err = templates.Parse(format)
		if err != nil {
			return err
		}
		if tmpl != nil {
			for _, value := range f.entries {
				img, ok := value.(*dockercompat.Image)
				if !ok {
					logrus.Warnf("%v failed to convert to  Image", value)
				}
				var b bytes.Buffer
				if err := tmpl.Execute(&b, img); err != nil {
					return err
				}
				if _, err = fmt.Fprintf(clicontext.App.Writer, b.String()+"\n"); err != nil {
					return err
				}
			}
		}
	}

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
