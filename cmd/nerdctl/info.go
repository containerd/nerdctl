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
	"io"
	"os"
	"strings"
	"text/template"

	"github.com/containerd/nerdctl/pkg/defaults"

	"github.com/containerd/nerdctl/pkg/infoutil"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/docker/go-units"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newInfoCommand() *cobra.Command {
	var infoCommand = &cobra.Command{
		Use:           "info",
		Args:          cobra.NoArgs,
		Short:         "Display system-wide information",
		RunE:          infoAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	infoCommand.Flags().StringP("format", "f", "", "Format the output using the given Go template, e.g, '{{json .}}'")
	infoCommand.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json"}, cobra.ShellCompDirectiveNoFileComp
	})
	return infoCommand
}

func infoAction(cmd *cobra.Command, args []string) error {
	var w io.Writer
	w = os.Stdout
	var (
		tmpl *template.Template
		err  error
	)
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	if format != "" {
		tmpl, err = parseTemplate(format)
		if err != nil {
			return err
		}
	}

	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	snapshotter, err := defaults.GetglobalString(cmd, "snapshotter")
	if err != nil {
		return err
	}
	cgroupManager, err := defaults.GetglobalString(cmd, "cgroup-manager")
	if err != nil {
		return err
	}
	info, err := infoutil.Info(ctx, client, snapshotter, cgroupManager)
	if err != nil {
		return err
	}

	if tmpl != nil {
		if err := tmpl.Execute(w, info); err != nil {
			return err
		}
		_, err = fmt.Fprintf(w, "\n")
		return err
	}

	namespace, err := defaults.GetglobalString(cmd, "namespace")
	if err != nil {
		return err
	}
	debug, err := defaults.GetglobalBool(cmd, "debug")
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "Client:\n")
	fmt.Fprintf(w, " Namespace:\t%s\n", namespace)
	fmt.Fprintf(w, " Debug Mode:\t%v\n", debug)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "Server:\n")
	fmt.Fprintf(w, " Server Version: %s\n", info.ServerVersion)
	// Storage Driver is not really Server concept for nerdctl, but mimics `docker info` output
	fmt.Fprintf(w, " Storage Driver: %s\n", info.Driver)
	fmt.Fprintf(w, " Logging Driver: %s\n", info.LoggingDriver)
	fmt.Fprintf(w, " Cgroup Driver: %s\n", info.CgroupDriver)
	fmt.Fprintf(w, " Cgroup Version: %s\n", info.CgroupVersion)
	fmt.Fprintf(w, " Plugins:\n")
	fmt.Fprintf(w, "  Log: %s\n", strings.Join(info.Plugins.Log, " "))
	fmt.Fprintf(w, "  Storage: %s\n", strings.Join(info.Plugins.Storage, " "))
	fmt.Fprintf(w, " Security Options:\n")
	for _, s := range info.SecurityOptions {
		m, err := strutil.ParseCSVMap(s)
		if err != nil {
			logrus.WithError(err).Warnf("unparsable security option %q", s)
			continue
		}
		name := m["name"]
		if name == "" {
			logrus.Warnf("unparsable security option %q", s)
			continue
		}
		fmt.Fprintf(w, "  %s\n", name)
		for k, v := range m {
			if k == "name" {
				continue
			}
			fmt.Fprintf(w, "   %s: %s\n", strings.Title(k), v)
		}
	}
	fmt.Fprintf(w, " Kernel Version: %s\n", info.KernelVersion)
	fmt.Fprintf(w, " Operating System: %s\n", info.OperatingSystem)
	fmt.Fprintf(w, " OSType: %s\n", info.OSType)
	fmt.Fprintf(w, " Architecture: %s\n", info.Architecture)
	fmt.Fprintf(w, " CPUs: %d\n", info.NCPU)
	fmt.Fprintf(w, " Total Memory: %s\n", units.BytesSize(float64(info.MemTotal)))
	fmt.Fprintf(w, " Name: %s\n", info.Name)
	fmt.Fprintf(w, " ID: %s\n", info.ID)

	fmt.Fprintln(w)
	if len(info.Warnings) > 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), strings.Join(info.Warnings, "\n"))
	}
	return nil
}
