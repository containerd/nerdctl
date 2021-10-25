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

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/nerdctl/pkg/idutil/imagewalker"
	"github.com/containerd/nerdctl/pkg/imageinspector"
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/docker/cli/templates"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newImageInspectCommand() *cobra.Command {
	var imageInspectCommand = &cobra.Command{
		Use:               "inspect [OPTIONS] IMAGE [IMAGE...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Display detailed information on one or more images.",
		Long:              "Hint: set `--mode=native` for showing the full output",
		RunE:              imageInspectAction,
		ValidArgsFunction: imageInspectShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	imageInspectCommand.Flags().String("mode", "dockercompat", `Inspect mode, "dockercompat" for Docker-compatible output, "native" for containerd-native output`)
	imageInspectCommand.RegisterFlagCompletionFunc("mode", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"dockercompat", "native"}, cobra.ShellCompDirectiveNoFileComp
	})
	imageInspectCommand.Flags().StringP("format", "f", "", "Format the output using the given Go template, e.g, '{{json .}}'")

	// #region platform flags
	imageInspectCommand.Flags().String("platform", "", "Inspect a specific platform") // not a slice, and there is no --all-platforms
	imageInspectCommand.RegisterFlagCompletionFunc("platform", shellCompletePlatforms)
	// #endregion

	return imageInspectCommand
}

func imageInspectAction(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return errors.Errorf("requires at least 1 argument")
	}

	var clientOpts []containerd.ClientOpt
	platform, err := cmd.Flags().GetString("platform")
	if err != nil {
		return err
	}
	if platform != "" {
		platformParsed, err := platforms.Parse(platform)
		if err != nil {
			return err
		}
		platformM := platforms.Only(platformParsed)
		clientOpts = append(clientOpts, containerd.WithDefaultPlatform(platformM))
	}
	client, ctx, cancel, err := newClient(cmd, clientOpts...)
	if err != nil {
		return err
	}
	defer cancel()

	mode, err := cmd.Flags().GetString("mode")
	if err != nil {
		return err
	}

	f := &imageInspector{
		mode: mode,
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
	for _, req := range args {
		n, err := walker.Walk(ctx, req)
		if err != nil {
			errs = append(errs, err)
		} else if n == 0 {
			errs = append(errs, errors.Errorf("no such object: %s", req))
		}
	}

	var tmpl *template.Template
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	switch format {
	case "":
		b, err := json.MarshalIndent(f.entries, "", "    ")
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(b))
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
				if _, err = fmt.Fprintf(cmd.OutOrStdout(), b.String()+"\n"); err != nil {
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

func imageInspectShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show image names
	return shellCompleteImageNames(cmd)
}
