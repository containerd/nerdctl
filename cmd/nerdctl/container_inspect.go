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

	"github.com/containerd/nerdctl/pkg/containerinspector"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/docker/cli/templates"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newContainerInspectCommand() *cobra.Command {
	var containerInspectCommand = &cobra.Command{
		Use:               "inspect [flags] CONTAINER [CONTAINER, ...]",
		Short:             "Display detailed information on one or more containers.",
		Long:              "Hint: set `--mode=native` for showing the full output",
		RunE:              containerInspectAction,
		ValidArgsFunction: containerInspectShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	containerInspectCommand.Flags().String("mode", "dockercompat", `Inspect mode, "dockercompat" for Docker-compatible output, "native" for containerd-native output`)
	containerInspectCommand.RegisterFlagCompletionFunc("mode", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"dockercompat", "native"}, cobra.ShellCompDirectiveNoFileComp
	})
	containerInspectCommand.Flags().StringP("format", "f", "", "Format the output using the given Go template, e.g, '{{json .}}'")
	return containerInspectCommand
}

func containerInspectAction(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return errors.Errorf("requires at least 1 argument")
	}

	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	mode, err := cmd.Flags().GetString("mode")
	if err != nil {
		return err
	}
	f := &containerInspector{
		mode: mode,
	}
	walker := &containerwalker.ContainerWalker{
		Client:  client,
		OnFound: f.Handler,
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
				c, ok := value.(*dockercompat.Container)
				if !ok {
					logrus.Warnf("%v failed to convert to  Container", value)
				}
				var b bytes.Buffer
				if err := tmpl.Execute(&b, c); err != nil {
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

func containerInspectShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show container names
	return shellCompleteContainerNames(cmd, nil)
}
